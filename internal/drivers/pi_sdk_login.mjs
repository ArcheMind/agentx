import { pathToFileURL } from "node:url";
import { createInterface } from "node:readline/promises";
import { stdin, stdout } from "node:process";

const sdkPath = process.env.AX_PI_SDK;
if (!sdkPath) {
  throw new Error("AX_PI_SDK is required");
}

const { ModelRuntime } = await import(pathToFileURL(sdkPath).href);
const runtime = await ModelRuntime.create();
const providers = runtime.getProviders()
  .filter((provider) => provider.auth.oauth)
  .sort((left, right) => left.name.localeCompare(right.name));

if (providers.length === 0) {
  throw new Error("Pi has no OAuth subscription providers available");
}

const input = createInterface({ input: stdin, output: stdout, terminal: true });
const choose = async (title, options) => {
  stdout.write(`\n${title}\n`);
  options.forEach((option, index) => stdout.write(`  ${index + 1}. ${option.label}\n`));
  for (;;) {
    const answer = (await input.question("Select a number: ")).trim();
    const index = Number.parseInt(answer, 10) - 1;
    if (Number.isInteger(index) && index >= 0 && index < options.length) return options[index].id;
    stdout.write("Enter a listed number.\n");
  }
};

try {
  const providerId = await choose("Select a Pi subscription provider:", providers.map((provider) => ({
    id: provider.id,
    label: provider.name,
  })));
  await runtime.login(providerId, "oauth", {
    prompt: async (prompt) => {
      if (prompt.type === "select") {
        return choose(prompt.message, prompt.options.map((option) => ({ id: option.id, label: option.label })));
      }
      return input.question(`${prompt.message}${prompt.placeholder ? ` (${prompt.placeholder})` : ""}: `);
    },
    notify: (event) => {
      if (event.type === "auth_url") {
        stdout.write(`\n${event.instructions ?? "Complete authentication in your browser."}\n${event.url}\n`);
      } else if (event.type === "device_code") {
        stdout.write(`\nOpen ${event.verificationUri} and enter code ${event.userCode}.\n`);
      } else {
        stdout.write(`\n${event.message}\n`);
      }
    },
  });
  stdout.write(`\nLogged in to ${providerId}.\n`);
} finally {
  input.close();
}
