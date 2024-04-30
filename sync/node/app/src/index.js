import { getPortPromise } from "portfinder";
import * as grpc from "@grpc/grpc-js";
import { HealthImplementation } from "grpc-health-check";
import protoLoader from "@grpc/proto-loader";
import protoBufSpec from "./generated/protobuf.json" with { type: "json" };
import stdioSpec from "./generated/grpc_stdio.json" with { type: "json" };
import Protobuf from "protobufjs";
import { ReflectionService } from "@grpc/reflection";
import Hyperswarm from "hyperswarm";
import Hypercore from "hypercore";
import Corestore from "corestore";
import Hyperbee from "hyperbee";
import { ed25519 } from "@noble/curves/ed25519";
import * as b58 from "multiformats/bases/base58";
import hypercoreCrypto from "hypercore-crypto";
import Protomux from "protomux";
import c from "compact-encoding";
import { setTraceFunction } from 'hypertrace'

let swarm;
let core;
let store;
let bee;

const heathCheckStatusMap = {
    "sync.Sync": "SERVING",
    "": "NOT_SERVING",
};
const root = new Protobuf.Root();

const SYNC_PROTOCOL = "lumeweb.portal.sync";

const DISCOVERED_BEES = new Map();

const decodeB64 = (str) => Buffer.from(str, "base64");
const baseToHex = (str) => toHex(Buffer.from(decodeB64(str)));
const fromHex = (str) => Buffer.from(str, "hex");
const toHex = (str) => Buffer.from(str).toString("hex");

function objectToLogEntry (obj) {
    const entry = obj.toJSON();

    entry.hash = baseToHex(entry.hash);
    entry.proof = toHex(entry.proof);
    if (entry.multihash) {
        const multihashRaw = decodeB64(entry.multihash);
        if (multihashRaw.length > 0) {
            entry.multihash = b58.encode(Buffer.from(multihashRaw));
        }
    } else {
        entry.multihash = "";
    }
    entry.key = baseToHex(entry.key.entropy);
    entry.size = Number(entry.size);

    for (const slab of entry.slabs) {
        slab.slab.key = toHex(slab.slab.key.entropy);
    }

    return entry;
}

function logEntryToObject (entry) {
    entry.hash = fromHex(entry.hash);
    entry.proof = fromHex(entry.proof);
    if (entry.multihash) {
        entry.multihash = b58.decode(entry.multihash);
    } else {
        entry.multihash = new Buffer();
    }

    entry.key = { entropy: fromHex(entry.key) };

    for (const slab of entry.slabs) {
        slab.slab.key = { entropy: fromHex(slab.slab.key) };
    }

    return root.lookupType("sync.FileMeta").fromObject(entry);
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
                const privateKey = request.privateKey;
                const pubKey = ed25519.getPublicKey(privateKey.slice(32));
                const keyPair = {
                    publicKey: pubKey,
                    secretKey: privateKey,
                };

                core = new Hypercore("./data", { keyPair });
                bee = new Hyperbee(core, { keyEncoding: "utf-8", valueEncoding: "json" });
                await bee.ready();

                store = new Corestore("./data");

                swarm = new Hyperswarm({ keyPair });
                swarm.join(bee.discoveryKey);
                swarm.join(hypercoreCrypto.hash(Buffer.from(SYNC_PROTOCOL)));
                swarm.on("connection", conn => bee.replicate(conn));
                swarm.on("connection", conn => store.replicate(conn));
                swarm.on("connection", (conn) => {
                    const mux = Protomux.from(conn);
                    mux.pair({ protocol: SYNC_PROTOCOL }, () => {
                        const sync = mux.createChannel({
                            protocol: SYNC_PROTOCOL,
                        });

                        const sendKey = sync.addMessage({
                            encoding: c.raw,
                            onmessage (m) {
                                if (c.raw.length === 32) {
                                    const dKey = toHex(m);
                                    if (!DISCOVERED_BEES.has(dKey)) {
                                        DISCOVERED_BEES.set(dKey, new Hyperbee(store.get({ key: m, sparse: true })));
                                    }
                                }
                            },
                        });

                        sync.open();
                        sendKey.send(core.key);
                    });
                });

                return { discoveryKey: bee.discoveryKey };
            },
            Update (call) {
                const req = root.lookupType("sync.UpdateRequest").fromObject(call.request);

                const obj = objectToLogEntry(req.data);

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
                const key = req.key;

                const resolveAlias = async (bee, key) => {
                    try {
                        const value = await bee.get(key);
                        if (value) {
                            if (typeof value === "string") {
                                // Value is an alias/pointer within the same Hyperbee, recursively search for the actual value
                                return await resolveAlias(bee, value);
                            } else {
                                // Value is not an alias/pointer, return it as is
                                return value;
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
                    const values = await searchHyperbees(key);
                    if (values.length > 0) {
                        const data = values.map(value => logEntryToObject(value).toObject());
                        return { data };
                    } else {
                        return { data: [] };
                    }
                } catch (err) {
                    console.error(err);
                    return { data: [] };
                }
            }
        })
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
