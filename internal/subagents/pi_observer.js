// AgentX Pi observer: records what each provider request actually carries into the Pi session.
// It never changes the payload, registers no tools and reads nothing beyond the provider request.
export default function (pi) {
  pi.on("before_provider_request", (event) => {
    const payload = event.payload ?? {};
    const record = { reasoning_effort: payload.reasoning?.effort ?? payload.reasoning_effort ?? null };
    pi.appendEntry("agentx-provider-request", record);
  });
}
