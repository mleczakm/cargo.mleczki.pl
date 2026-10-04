import { test } from "node:test";
import assert from "node:assert/strict";
import { acceptEmail, deliverQueue } from "./index.js";

const address = "0123456789abcdef0123456789abcdef@cargo.mleczki.pl";
const expected = "powiadomienia@alior.pl";

function email(from = expected, to = address, headerFrom = from) {
  const raw = `From: ${headerFrom}\r\nTo: ${to}\r\nSubject: Uznanie rachunku - Kwota: 12,00 PLN\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nTytuł: K7PX\r\n`;
  return {
    from, to, rawSize: Buffer.byteLength(raw), raw: new Blob([raw]).stream(),
    rejected: "", setReject(reason) { this.rejected = reason; },
  };
}

function environment(checkFrom = "1") {
  const sent = [];
  return {
    BANK_MAIL_ADDRESS: address, BANK_MAIL_FROM: expected, BANK_MAIL_CHECK_FROM: checkFrom,
    BANK_MAIL_QUEUE: { async send(value) { sent.push(value); } }, sent,
  };
}

test("only the configured recipient and sender are accepted by default", async () => {
  const env = environment();
  const valid = email();
  await acceptEmail(valid, env);
  assert.equal(valid.rejected, "");
  assert.equal(env.sent.length, 1);
  assert.match(env.sent[0].id, /^[0-9a-f]{64}$/);
  assert.equal(env.sent[0].recipient, address);

  const wrongSender = email("other@example.com");
  await acceptEmail(wrongSender, env);
  assert.equal(wrongSender.rejected, "Unexpected sender");
  const spoofedHeader = email(expected, address, "other@example.com");
  await acceptEmail(spoofedHeader, env);
  assert.equal(spoofedHeader.rejected, "Unexpected sender");
  const wrongRecipient = email(expected, "other@cargo.mleczki.pl");
  await acceptEmail(wrongRecipient, env);
  assert.equal(wrongRecipient.rejected, "Unknown recipient");
  assert.equal(env.sent.length, 1);
});

test("sender check may be disabled for controlled tests", async () => {
  const env = environment("0");
  await acceptEmail(email("tester@example.com"), env);
  assert.equal(env.sent.length, 1);
});

test("failed application delivery is retried", async () => {
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async () => ({ ok: false, status: 503 });
  try {
    let retried = false;
    const message = { body: { id: "test" }, attempts: 1, ack() { assert.fail("must not ack"); },
      retry() { retried = true; } };
    await deliverQueue({ messages: [message] }, { APP_URL: "https://cargo.mleczki.pl", BANK_MAIL_WEBHOOK_SECRET: "secret" });
    assert.equal(retried, true);
  } finally {
    globalThis.fetch = previousFetch;
  }
});

test("successful delivery is signed and acknowledged", async () => {
  const previousFetch = globalThis.fetch;
  const secret = "test-secret";
  globalThis.fetch = async (url, options) => {
    assert.equal(url, "https://cargo.mleczki.pl/api/bank-mail");
    const timestamp = options.headers["X-Cargo-Bank-Timestamp"];
    const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(secret),
      { name: "HMAC", hash: "SHA-256" }, false, ["verify"]);
    const signature = Uint8Array.from(options.headers["X-Cargo-Bank-Signature"].match(/../g), (v) => parseInt(v, 16));
    assert.equal(await crypto.subtle.verify("HMAC", key, signature,
      new TextEncoder().encode(`${timestamp}\n${options.body}`)), true);
    return { ok: true, status: 204 };
  };
  try {
    let acknowledged = false;
    const message = { body: { id: "test" }, attempts: 1, ack() { acknowledged = true; },
      retry() { assert.fail("must not retry"); } };
    await deliverQueue({ messages: [message] }, { APP_URL: "https://cargo.mleczki.pl", BANK_MAIL_WEBHOOK_SECRET: secret });
    assert.equal(acknowledged, true);
  } finally {
    globalThis.fetch = previousFetch;
  }
});
