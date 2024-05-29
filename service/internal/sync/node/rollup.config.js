import json from "@rollup/plugin-json";
import commonjs from "@rollup/plugin-commonjs";
import { nodeResolve } from "@rollup/plugin-node-resolve";

export default {
  input: "src/index.js",
  output: {
    file: "app/app/app/bundle.js",
    format: "cjs",
  },
  plugins: [
    json(),
    nodeResolve(),
    commonjs({
        ignoreDynamicRequires: true,
    }),
  ],
};
