import { defineExtension } from "@agentflow/extensions";

export default defineExtension({
  async run(ctx) {
    return {
      message: ctx.with.message,
      status: "ok",
    };
  },
});
