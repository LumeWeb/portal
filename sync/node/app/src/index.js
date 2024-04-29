import { getPortPromise } from "portfinder";
import * as grpc from "@grpc/grpc-js";
import { HealthImplementation } from "grpc-health-check";
import protoLoader from "@grpc/proto-loader";
import protoBufSpec from "./generated/protobuf.json" with { type: "json" };
import Protobuf from "protobufjs";
import { ReflectionService } from "@grpc/reflection";
import Hyperswarm from "hyperswarm";
import Corestore from "corestore";
import Hyperbee from "hyperbee";
import { ed25519 } from "@noble/curves/ed25519";

let swarm;
let store;
let bee;

const heathCheckStatusMap = {
    "sync.Sync": "SERVING",
    "": "NOT_SERVING",
};

async function main () {
    let foundPort;
    try {
        foundPort = await getPortPromise();
    } catch (err) {
        console.error(err);
        return;
    }

    const root = new Protobuf.Root();
    root.addJSON(protoBufSpec.nested).resolveAll();
    const packageDefinition = await protoLoader.loadFileDescriptorSetFromObject(root.toDescriptor());
    const syncPackage = grpc.loadPackageDefinition(packageDefinition);

    const healthImpl = new HealthImplementation(heathCheckStatusMap);
    const reflection = new ReflectionService(packageDefinition);

    const server = new grpc.Server();
    server.addService(syncPackage.sync.Sync.service,
        prepareServiceImpl({
            async Init (request) {
                const privateKey = request.privateKey;
                const pubKey = ed25519.getPublicKey(privateKey);
                const keyPair = {
                    publicKey: pubKey,
                    secretKey: Buffer.concat([pubKey, privateKey]),
                };

                store = new Corestore("./data", { primaryKey: privateKey });
                bee = new Hyperbee(store.get({ name: "default" }), { keyEncoding: "utf-8", valueEncoding: "json" });
                await bee.ready();

                swarm = new Hyperswarm({ keyPair });
                swarm.join(bee.discoveryKey);
                swarm.on("connection", conn => bee.replicate(conn));

                return {};
            },
            Update (request) {
                return {};
            }
        }),
    );
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
            const response = await func(call.request);
            callback(null, response);
        } catch (err) {
            callback(err, null);
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
