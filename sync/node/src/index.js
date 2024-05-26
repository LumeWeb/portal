import { getPortPromise } from "portfinder";
import * as grpc from "@grpc/grpc-js";
import { HealthImplementation } from "grpc-health-check";
import protoLoader from "@grpc/proto-loader";
import protoBufSpec from "./generated/protobuf.json" with { type: "json" };
import stdioSpec from "./generated/grpc_stdio.json" with { type: "json" };
import Protobuf from "protobufjs";
import { ReflectionService } from "@grpc/reflection";
import Hyperswarm from "hyperswarm";
import Corestore from "corestore";
import Hyperbee from "hyperbee";
import { ed25519 } from "@noble/curves/ed25519";
import * as b58 from "multiformats/bases/base58";
import hypercoreCrypto from "hypercore-crypto";
import Protomux from "protomux";
import c from "compact-encoding";
import b4a from "b4a";
import { setTraceFunction } from "hypertrace";
import Autobee from "./autobee.js";
let swarm;
let store;
let bee;
let dataDir

const heathCheckStatusMap = {
    "sync.Sync": "SERVING",
    "": "NOT_SERVING",
};
const root = new Protobuf.Root();

const SYNC_PROTOCOL = "lumeweb.portal.sync";

const DISCOVERED_BEES = new Map();

const decodeB64 = (str) => Buffer.from(str, "base64");
const encodeB64 = (str) => Buffer.from(str).toString("base64");
const baseToHex = (str) => toHex(Buffer.from(decodeB64(str)));
const fromHex = (str) => Buffer.from(str, "hex");
const toHex = (str) => Buffer.from(str).toString("hex");

const encoding = { keyEncoding: "utf-8", valueEncoding: "json" };

setTraceFunction(({ id, caller, object, parentObject }) => {
    console.log({
        id,
        caller,
        object,
        parentObject,
    });
});

function objectToLogEntry(obj, raw) {
    const entry = obj.toJSON();

    entry.hash = baseToHex(entry.hash);
    entry.proof = baseToHex(entry.proof);
    if (entry.multihash) {
        const multihashRaw = decodeB64(entry.multihash);
        if (multihashRaw.length > 0) {
            entry.multihash = b58.encode(Buffer.from(multihashRaw));
        }
    } else {
        entry.multihash = "";
    }
    entry.key = `key:${baseToHex(entry.key.entropy)}`;
    entry.size = Number(entry.size);

    if (Array.isArray(entry.slabs)) {
        for (const [index, slab] of entry.slabs.entries()) {
            slab.slab.key = `key:${baseToHex(slab.slab.key.entropy)}`;
            if (Array.isArray(slab.slab.shards)) {
                for (const [shardIndex, shard] of slab.slab.shards.entries()) {
                    shard.root = `h:${baseToHex(shard.root)}`;
                    shard.latestHost = `ed25519:${baseToHex(shard.latestHost)}`;

                    const rawShard = raw.data.slabs[index].slab.shards[shardIndex];
                    const transformedContracts = {};

                    if (Array.isArray(rawShard.contractSet)) {
                        // Transform the contractSet array
                        rawShard.contractSet.forEach(contractSet => {
                            transformedContracts[contractSet.key] = contractSet.value.contracts.map(contract => `fcid:${toHex(contract.id)}`);
                        });
                    }

                    shard.contracts = transformedContracts;
                    delete shard.contractSet;
                }
            }
        }
    }

    return entry;
}

function logEntryToObject (entry) {
    entry.hash = fromHex(entry.hash);
    entry.proof = fromHex(entry.proof);
    if (entry.multihash) {
        entry.multihash = b58.decode(entry.multihash).toString("base64");
    } else {
        entry.multihash = Buffer.from([]);
    }
    entry.key = { entropy: fromHex(entry.key.slice(4)) };

    if (Array.isArray(entry.slabs)) {
        for (const slab of entry.slabs) {
            slab.slab.key = { entropy: fromHex(slab.slab.key.slice(4)) };

            if (Array.isArray(slab.slab.shards)) {
                for (const shard of slab.slab.shards) {
                    shard.root = `${encodeB64(fromHex(shard.root.slice(2)))}`;
                    shard.latestHost = `${encodeB64(fromHex(shard.latestHost.slice(8)))}`;

                    const transformedContracts = {};
                    for (const [key, contracts] of Object.entries(shard.contracts)) {
                        transformedContracts[key] = {
                            key: key,
                            value: {
                                contracts: contracts.map(contract => ({
                                    id: fromHex(contract.slice(5)),
                                })),
                            },
                        };
                    }
                    shard.contractSet = Object.values(transformedContracts);
                    delete shard.contracts;
                }
            }
        }
    }

    return entry;
}

async function main () {
    let foundPort;
    try {
        foundPort = await getPortPromise();
    } catch (err) {
        console.error(err);
        return;
    }

    root.addJSON(protoBufSpec.nested).resolveAll();
    root.addJSON(stdioSpec.nested).resolveAll();
    const packageDefinition = await protoLoader.loadFileDescriptorSetFromObject(root.toDescriptor());
    const syncPackage = grpc.loadPackageDefinition(packageDefinition);

    const healthImpl = new HealthImplementation(heathCheckStatusMap);
    const reflection = new ReflectionService(packageDefinition);

    const server = new grpc.Server();
    server.addService(syncPackage.sync.Sync.service,
        prepareServiceImpl({
            async Init (call) {
                const request = call.request;
                const bootstrap = request.bootstrap;
                const logPrivateKey = request.logPrivateKey;
                const nodePrivateKey = request.nodePrivateKey;
                const pubKey = ed25519.getPublicKey(nodePrivateKey.slice(0, 32));
                const keyPair = { publicKey: pubKey, secretKey: nodePrivateKey };

                dataDir = request.dataDir;

                store = new Corestore(dataDir);
                const logPublicKey = ed25519.getPublicKey(logPrivateKey.slice(0, 32));

                if (bootstrap) {
                    const bootstrapCore = store.get({ keyPair: { publicKey: logPublicKey, secretKey: logPrivateKey } });
                    await bootstrapCore.ready();
                    await bootstrapCore.setUserData('autobase/local', bootstrapCore.key);

                    await store.get({ keyPair }).ready();
                }

                bee = new Autobee(store, logPublicKey, {
                    apply: async (batch, view, base) => {
                        // Add .addWriter functionality
                        for (const node of batch) {
                            const op = node.value
                            if (op.type === 'addWriter') {
                                const key = b4a.from(op.key, "hex");
                                await base.addWriter(key);
                                if (op.bootstrap) {
                                    await base.system.add(key, { isIndexer: true, isPending: false });
                                }
                            }

                            if (op.type === 'removeWriter') {
                                await base.removeWriter(b4a.from(op.key, 'hex'))
                            }
                        }

                        // Pass through to Autobee's apply
                        await Autobee.apply(batch, view, base)
                    },

                    // Set encodings for autobase/hyperbee
                    ...encoding
                })
                    // Print any errors from apply() etc
                    .on('error', console.error)

                if (bootstrap) {
                    await bee.addNode(b4a.from(pubKey).toString("hex"), true);
                }
                await bee.update();

                swarm = new Hyperswarm({ keyPair });
                swarm.join(bee.discoveryKey);
                swarm.join(hypercoreCrypto.hash(Buffer.from(SYNC_PROTOCOL)));

                const peerHandler = (conn) => {
                    const sync = Protomux.from(conn).createChannel({
                        protocol: SYNC_PROTOCOL,
                    });

                    const sendKey = sync.addMessage({
                        encoding: c.raw,
                        onmessage (m) {
                            if (m.length === 32) {
                                const dKey = toHex(m);
                                if (!DISCOVERED_BEES.has(dKey)) {
                                    DISCOVERED_BEES.set(dKey, new Hyperbee(store.get({ key: m }), { ...encoding }));
                                }
                            }
                        },
                    });

                    sync.open();
                    sendKey.send(bee.key);
                };

                swarm.on("connection", conn => bee.replicate(conn));
                swarm.on("connection", (conn) => {
                    const mux = Protomux.from(conn);
                    if (b4a.equals(swarm.keyPair.publicKey, mux.stream.remotePublicKey)) {
                        return;
                    }
                    if (mux.stream.isInitiator) {
                        peerHandler(conn);
                        return;
                    }

                    mux.pair({ protocol: SYNC_PROTOCOL }, peerHandler.bind(null, conn));
                });

                async function exit () {
                    await swarm.destroy();
                }

                process.on("SIGTERM", exit);
                process.on("exit", exit);

                return { logKey: bee.key };
            },
            async Update (call) {
                const req = root.lookupType("sync.UpdateRequest").fromObject(call.request);

                if (!bee.writable) {
                    throw new Error("log is not writable");
                }

                const obj = objectToLogEntry(req.data, call.request);

                const aliases = obj.aliases || [];
                delete obj.aliases;

                bee.put(obj.hash, obj);

                for (const alias of aliases) {
                    bee.put(alias, obj.hash);
                }

                return {};
            },
            async Query (call) {
                const req = root.lookupType("sync.QueryRequest").fromObject(call.request);
                const keys = req.keys;

                const resolveAlias = async (bee, key) => {
                    try {
                        const value = await bee.get(key);
                        if (value?.value) {
                            if (typeof value?.value === "string") {
                                // Value is an alias/pointer within the same Hyperbee, recursively search for the actual value
                                return await resolveAlias(bee, value?.value);
                            } else {
                                // Value is not an alias/pointer, return it as is
                                return value?.value;
                            }
                        } else {
                            return null;
                        }
                    } catch (err) {
                        return null;
                    }
                };

                const searchHyperbees = async (key) => {
                    const foundEntries = [];

                    // Search the local Hyperbee
                    const localValue = await resolveAlias(bee, key);
                    if (localValue) {
                        foundEntries.push(localValue);
                    }

                    // Search the discovered Hyperbees
                    for (const bee of DISCOVERED_BEES.values()) {
                        await bee.ready();

                        const remoteValue = await resolveAlias(bee, key);
                        if (remoteValue) {
                            foundEntries.push(remoteValue);
                        }
                    }

                    return foundEntries;
                };

                try {
                    const uniqueEntries = [];
                    const data = [];

                    for (const key of keys) {
                        const values = await searchHyperbees(key);
                        if (values.length > 0) {
                            const entries = values.map(value => logEntryToObject(value));
                            entries.forEach(entry => {
                                // Check if the entry is already present in uniqueEntries using deepEqual()
                                const isDuplicate = uniqueEntries.some(uniqueEntry => deepEqual(entry, uniqueEntry));
                                if (!isDuplicate) {
                                    uniqueEntries.push(entry);
                                    data.push(entry);
                                }
                            });
                        }
                    }

                    return { data };
                } catch (err) {
                    console.error(err);
                    return { data: [] };
                }
            },
            async UpdateNodes (call) {
                if (!bee.writable) {
                    throw new Error("log is not writable");
                }
                const req = root.lookupType("sync.UpdateNodesRequest").fromObject(call.request);

                const existingNodes =  [];

                for(const writer of bee.activeWriters) {
                    existingNodes.push(b4a.from(writer.core.key).toString("hex"));
                }

                // Add missing nodes to bee
                for (const node of req.nodes) {
                    const keyStr = b4a.from(node).toString("hex");
                    if (!existingNodes.includes(keyStr)) {
                        await bee.addNode(keyStr);
                    }
                }

                return {};
            },
            async RemoveNode(call) {
                const req = root.lookupType("sync.RemoveNodeRequest").fromObject(call.request);

                await bee.removeNode(b4a.from(req.node).toString("hex"));

                return {};
            }
        }),
    );
    server.addService(syncPackage.plugin.GRPCStdio.service, prepareServiceImpl({
        StreamStdio (call) {

        },
    }));

    healthImpl.addToServer(server);
    reflection.addToServer(server);
    server.bindAsync(`127.0.0.1:${foundPort}`, grpc.ServerCredentials.createInsecure(), () => {
        console.log("1|1|tcp|127.0.0.1:%d|grpc", foundPort);
    });
}

main();

function prepareRpcMethod (func) {
    return async (call, callback) => {
        try {
            const response = await func(call);
            callback?.(null, response);
        } catch (err) {
            callback?.(err, null);
        }
    };
}

function prepareServiceImpl (service) {
    const impl = {};
    for (const method of Object.keys(service)) {
        impl[method] = prepareRpcMethod(service[method]);
    }

    return impl;
}
