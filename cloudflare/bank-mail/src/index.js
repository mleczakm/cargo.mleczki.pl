import PostalMime from "postal-mime";
import { mailForMimeParsing } from "../shared/mime.js";
import { readBankMail, postSignedBankMail } from "../shared/transport.js";

const encoder = new TextEncoder();
const maxRawSize = 1024 * 1024;
const maxPayloadSize = 96 * 1024;

function exactAddress(value) {
  return String(value || "").trim().toLowerCase();
}

async function sha256Hex(buffer) {
  const digest = await crypto.subtle.digest("SHA-256", buffer);
  return [...new Uint8Array(digest)].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

export async function acceptEmail(message, env) {
  if (exactAddress(message.to) !== exactAddress(env.BANK_MAIL_ADDRESS)) {
    message.setReject("Unknown recipient");
    return;
  }
  if (message.rawSize > maxRawSize) {
    message.setReject("Bank notification too large");
    return;
  }
  const raw = await readBankMail(message, { maxSize: maxRawSize, sizeReason: "Bank notification too large" });
  if (!raw) return;
  const parsed = await PostalMime.parse(mailForMimeParsing(raw));
  // The switch is only for controlled tests. Production defaults to checking both
  // the SMTP envelope sender and the visible From header.
  if (env.BANK_MAIL_CHECK_FROM !== "0") {
    const expected = exactAddress(env.BANK_MAIL_FROM);
    if (!expected || exactAddress(message.from) !== expected || exactAddress(parsed.from?.address) !== expected) {
      message.setReject("Unexpected sender");
      return;
    }
  }
  const payload = {
    id: await sha256Hex(raw),
    received_at: new Date().toISOString(),
    recipient: exactAddress(message.to),
    mail_from: exactAddress(message.from),
    subject: parsed.subject || "",
    body: parsed.html || parsed.text || "",
  };
  if (encoder.encode(JSON.stringify(payload)).length > maxPayloadSize) {
    message.setReject("Bank notification content too large");
    return;
  }
  await env.BANK_MAIL_QUEUE.send(payload);
}

export async function deliverQueue(batch, env) {
  for (const message of batch.messages) {
    const body = JSON.stringify(message.body);
    const timestamp = new Date().toISOString();
    try {
      await postSignedBankMail({ url: `${env.APP_URL}/api/bank-mail`, secret: env.BANK_MAIL_WEBHOOK_SECRET, body, contentType: "application/json", timestamp, timestampHeader: "X-Cargo-Bank-Timestamp", signatureHeader: "X-Cargo-Bank-Signature" });
      message.ack();
    } catch (error) {
      console.error("Bank notification delivery failed", message.body.id, String(error));
      message.retry({ delaySeconds: Math.min(1800, 30 * 2 ** Math.min(message.attempts - 1, 6)) });
    }
  }
}

export default {
  email: acceptEmail,
  queue: deliverQueue,
};
