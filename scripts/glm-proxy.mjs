#!/usr/bin/env node
// Minimal OpenAI-compatible SSE proxy for SHEYTAN e2e testing.
// Backed by the z-ai CLI (chat completions). Supports:
//   - /v1/chat/completions (streaming SSE with content deltas)
//   - /v1/models
// The proxy injects the tool-calling protocol into the system prompt and
// parses the model's <<TOOL_CALL {json}>> replies into real tool_calls.

import http from "http";
import { spawnSync } from "child_process";

const PORT = parseInt(process.env.PROXY_PORT || "8177", 10);

function callZAI(system, user) {
  return new Promise((resolve, reject) => {
    const r = spawnSync("z-ai", ["chat", "-p", user, "-s", sysPrompt(system)], {
      encoding: "utf8",
      timeout: 180000,
      maxBuffer: 32 * 1024 * 1024,
    });
    if (r.status !== 0) {
      return reject(new Error("z-ai chat failed: " + (r.stderr || "").slice(0, 400)));
    }
    let out = (r.stdout || "").trim();
    // z-ai CLI may print init lines then a JSON blob; find the content field.
    try {
      const start = out.indexOf("{");
      const parsed = JSON.parse(out.slice(start));
      if (parsed.choices && parsed.choices[0] && parsed.choices[0].message) {
        out = parsed.choices[0].message.content;
      }
    } catch {
      // plain text reply — use as-is
    }
    resolve(out);
  });
}

function sse(res, obj) {
  res.write(`data: ${JSON.stringify(obj)}\n\n`);
}

function sysPrompt(system) {
  return system + `

You have tools available. To call one, reply with EXACTLY one line:
<<TOOL_CALL {"tool":"<name>","args":{...}}>>
After each tool result you may call another tool or give the final answer as
plain text (no tool call). Keep final answers short for tests.`;
}

// repairJSON fixes the classic LLM JSON mistakes: raw newlines/tabs inside
// string literals (invalid JSON), trailing commas, and smart quotes.
function repairJSON(s) {
  let out = "";
  let inStr = false;
  let esc = false;
  for (const ch of s) {
    if (esc) {
      out += ch;
      esc = false;
      continue;
    }
    if (ch === "\\") {
      out += ch;
      esc = true;
      continue;
    }
    if (ch === '"') {
      inStr = !inStr;
      out += ch;
      continue;
    }
    if (inStr && ch === "\n") {
      out += "\\n";
      continue;
    }
    if (inStr && ch === "\r") {
      continue;
    }
    if (inStr && ch === "\t") {
      out += "\\t";
      continue;
    }
    out += ch;
  }
  out = out.replace(/,\s*([}\]])/g, "$1");
  return out;
}

const server = http.createServer(async (req, res) => {
  if (req.url.endsWith("/models")) {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ object: "list", data: [{ id: "glm-proxy" }] }));
    return;
  }
  if (req.url.endsWith("/chat/completions")) {
    let body = "";
    for await (const chunk of req) body += chunk;
    let parsed;
    try {
      parsed = JSON.parse(body);
    } catch {
      res.writeHead(400);
      res.end("bad json");
      return;
    }
    // Compose the conversation into a single prompt for the CLI.
    const sys = (parsed.messages || [])
      .filter((m) => m.role === "system")
      .map((m) => m.content)
      .join("\n\n")
      .slice(0, 60000);
    const convo = (parsed.messages || [])
      .filter((m) => m.role !== "system")
      .map((m) => {
        if (m.role === "tool") {
          return `TOOL RESULT (${m.name || "tool"}):\n${m.content}`;
        }
        if (m.role === "assistant") {
          return `ASSISTANT: ${m.content || "(tool call)"}`;
        }
        return `USER: ${m.content}`;
      })
      .join("\n\n");
    const toolList = (parsed.tools || [])
      .map((t) => `- ${t.function.name}: ${t.function.description}`)
      .join("\n");

    let reply = "";
    try {
      reply = await callZAI(
        sys + (toolList ? `\n\nTOOLS:\n${toolList}` : ""),
        convo + "\n\nRespond now."
      );
    } catch (e) {
      res.writeHead(500);
      res.end(String(e));
      return;
    }

    // Parse tool-call protocol out of the reply.
    let toolCall = null;
    let content = reply;
    const m = reply.match(/<<TOOL_CALL\s*(\{[\s\S]*?\})\s*>>/);
    if (m) {
      try {
        const call = JSON.parse(repairJSON(m[1]));
        toolCall = { tool: call.tool, args: call.args || {} };
        content = reply.slice(0, m.index).trim();
      } catch {
        // malformed — treat as content
      }
    }

    res.writeHead(200, { "Content-Type": "text/event-stream" });
    if (content) {
      sse(res, {
        choices: [{ delta: { role: "assistant", content } }],
      });
    }
    if (toolCall) {
      sse(res, {
        choices: [
          {
            delta: {
              tool_calls: [
                {
                  index: 0,
                  id: "call_" + Date.now(),
                  type: "function",
                  function: {
                    name: toolCall.tool,
                    arguments: JSON.stringify(toolCall.args),
                  },
                },
              ],
            },
          },
        ],
      });
      sse(res, { choices: [{ delta: {}, finish_reason: "tool_calls" }] });
    } else {
      sse(res, { choices: [{ delta: {}, finish_reason: "stop" }] });
    }
    res.write("data: [DONE]\n\n");
    res.end();
    return;
  }
  res.writeHead(404);
  res.end("not found");
});

server.listen(PORT, "127.0.0.1", () => {
  console.log(`glm proxy on http://127.0.0.1:${PORT}/v1`);
});
