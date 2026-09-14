// Evaluates one expression in the test instance's page over the Chrome
// DevTools Protocol and prints the JSON result. Promises are awaited. Any
// failure prints {"error": "..."} on stdout and exits non-zero.
//
//   node cdp.mjs <port> <expression>
const [port, expression] = process.argv.slice(2);

function fail(message, code = 1) {
  console.log(JSON.stringify({ error: String(message) }));
  process.exit(code);
}

try {
  const targets = await fetch(`http://127.0.0.1:${port}/json/list`).then((r) => r.json());
  const page = targets.find((t) => t.type === "page" && t.url.startsWith("http://127.0.0.1:"));
  if (!page) fail("no page target yet", 2);

  const ws = new WebSocket(page.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = () => reject(new Error("websocket error"));
  });
  const reply = await new Promise((resolve) => {
    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      if (message.id === 1) resolve(message);
    };
    ws.send(JSON.stringify({
      id: 1,
      method: "Runtime.evaluate",
      params: { expression, awaitPromise: true, returnByValue: true },
    }));
  });
  ws.close();

  const result = reply.result ?? {};
  if (result.exceptionDetails) {
    const details = result.exceptionDetails;
    fail(details.exception?.description ?? details.exception?.value ?? details.text);
  }
  console.log(JSON.stringify(result.result?.value ?? null));
  process.exit(0);
} catch (error) {
  fail(error?.message ?? error, 2);
}
