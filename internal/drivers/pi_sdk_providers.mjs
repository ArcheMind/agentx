import { pathToFileURL } from "node:url";

const sdkPath = process.env.AX_PI_SDK;
if (!sdkPath) {
  throw new Error("AX_PI_SDK is required");
}

const { ModelRuntime } = await import(pathToFileURL(sdkPath).href);
const runtime = await ModelRuntime.create();
const providerIDs = runtime.getProviders()
  .filter((provider) => runtime.hasConfiguredAuth(provider.id))
  .map((provider) => provider.id)
  .sort();

process.stdout.write(providerIDs.join("\n"));
