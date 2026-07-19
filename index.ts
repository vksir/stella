import { AuthStorage, createAgentSession, ModelRegistry, SessionManager } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const agentDir = "D:\\Code\\MyProject\\stella\\.workspace\\.pi";
const cwd = "D:\\Code\\MyProject\\stella\\.workspace\\cwd";

// AuthStorage 和 ModelRegistry 也需要指向自定义目录
const authStorage = AuthStorage.create(`${agentDir}/auth.json`);
const modelRegistry = ModelRegistry.create(authStorage, `${agentDir}/models.json`);

const { session } = await createAgentSession({
  sessionManager: SessionManager.inMemory(),
  authStorage,
  modelRegistry,
  agentDir,
  cwd,
});

session.subscribe((event) => {
  if (event.type === "message_update" && event.assistantMessageEvent.type === "text_delta") {
    process.stdout.write(event.assistantMessageEvent.delta);
  }
});

await session.prompt("What files are in the current directory?");
