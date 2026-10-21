// AgentX Pi observer: records what each provider request actually carries into the Pi session.
// It never changes the payload, registers no tools and reads only the role file AgentX wrote.
import { readFileSync } from "node:fs";

export default function (pi) {
  const rolePath = process.env.AGENTX_PI_ROLE_FILE;
  const role = rolePath ? readFileSync(rolePath, "utf8") : undefined;
  pi.on("before_provider_request", (event) => {
    const payload = event.payload ?? {};
    const record = { reasoning_effort: payload.reasoning?.effort ?? payload.reasoning_effort ?? null };
    if (role !== undefined) {
      record.role_included = JSON.stringify(payload).includes(JSON.stringify(role).slice(1, -1));
    }
    pi.appendEntry("agentx-provider-request", record);
  });
}
