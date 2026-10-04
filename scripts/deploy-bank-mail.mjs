// Provision Email Routing and the queue, then deploy the Worker. Run only after
// the application deployment so its signed endpoint is available.
import { readFile, writeFile, unlink } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const configPath = path.join(root, "cloudflare/bank-mail/wrangler.generated.jsonc");
const baseConfigPath = path.join(root, "cloudflare/bank-mail/wrangler.jsonc");
const envPath = process.env.ENV_FILE_PATH || path.join(root, ".env.production");

function fail(message) {
  throw new Error(message);
}

function parseEnv(text) {
  return Object.fromEntries(text.split(/\r?\n/).filter((line) => /^[A-Z][A-Z0-9_]*=/.test(line))
    .map((line) => [line.slice(0, line.indexOf("=")), line.slice(line.indexOf("=") + 1)]));
}

const appEnv = parseEnv(await readFile(envPath, "utf8"));
const address = appEnv.BANK_MAIL_ADDRESS;
const key = appEnv.BANK_MAIL_WEBHOOK_SECRET;
const from = appEnv.BANK_MAIL_FROM || "powiadomienia@alior.pl";
const checkFrom = appEnv.BANK_MAIL_CHECK_FROM || "1";
const zoneId = process.env.CLOUDFLARE_ZONE_ID;
const token = process.env.CLOUDFLARE_API_TOKEN;
if (!/^[a-f0-9]{32}@cargo\.mleczki\.pl$/.test(address || "")) fail("BANK_MAIL_ADDRESS must be the generated 32-hex address at cargo.mleczki.pl");
if (!/^[a-f0-9]{64}$/.test(key || "")) fail("BANK_MAIL_WEBHOOK_SECRET must be a generated 64-hex key");
if (!["0", "1"].includes(checkFrom)) fail("BANK_MAIL_CHECK_FROM must be 0 or 1");
if (!from || !zoneId || !token) fail("Cloudflare zone, token and bank sender are required");

async function api(method, endpoint, body, allowMissing = false) {
  const response = await fetch(`https://api.cloudflare.com/client/v4${endpoint}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (allowMissing && response.status === 404) return null;
  if (!response.ok || !data.success) fail(`Cloudflare API ${method} ${endpoint}: ${response.status} ${JSON.stringify(data.errors || [])}`);
  return data;
}

function wrangler(args, input) {
  const result = spawnSync(path.join(root, "node_modules/.bin/wrangler"),
    [...args, "--config", configPath], { cwd: root, env: process.env, input, encoding: "utf8", stdio: input === undefined ? "inherit" : ["pipe", "inherit", "inherit"] });
  if (result.status !== 0) fail(`wrangler ${args.join(" ")} failed (${result.status})`);
}

// The DNS deployment already has the zone ID. Cloudflare returns its owning
// account ID with the zone details, so no second GitHub secret is needed.
const zone = await api("GET", `/zones/${zoneId}`);
if (zone.result?.name !== "mleczki.pl") fail("CLOUDFLARE_ZONE_ID does not belong to mleczki.pl");
const accountId = zone.result.account?.id;
if (!accountId) fail("Cloudflare zone response did not include its account ID");

// The apex mleczki.pl keeps its Zoho MX records, so Email Routing must only be
// active on the cargo.mleczki.pl subdomain. Never enable or change it on the apex.
const routing = await api("GET", `/zones/${zoneId}/email/routing`);
const subdomain = routing.result?.subdomains?.find((item) => item.name === "cargo.mleczki.pl");
if (!subdomain?.enabled || subdomain.status !== "ready") {
  fail("Email Routing is not ready for the cargo.mleczki.pl subdomain; enable that subdomain in the Cloudflare dashboard (Email > Email Routing > Settings > Subdomains)");
}

const queues = await api("GET", `/accounts/${accountId}/queues`);
for (const queueName of ["cargo-bank-mail", "cargo-bank-mail-dlq"]) {
  if (!queues.result.some((queue) => queue.queue_name === queueName)) {
    await api("POST", `/accounts/${accountId}/queues`, { queue_name: queueName });
  }
}

const config = JSON.parse(await readFile(baseConfigPath, "utf8"));
config.account_id = accountId;
config.vars = { APP_URL: "https://cargo.mleczki.pl" };
try {
  // Omit Wrangler's addresses field: its reconciliation API requires the
  // account-level Email Routing Account Rules Read permission, which account
  // tokens cannot select. The zone-level rule API below uses our scoped token.
  await writeFile(configPath, JSON.stringify(config, null, 2), { mode: 0o600 });
  wrangler(["deploy"]);
  for (const [name, value] of Object.entries({
    BANK_MAIL_ADDRESS: address,
    BANK_MAIL_WEBHOOK_SECRET: key,
    BANK_MAIL_FROM: from,
    BANK_MAIL_CHECK_FROM: checkFrom,
  })) {
    wrangler(["secret", "put", name], value);
  }
  const rules = await api("GET", `/zones/${zoneId}/email/routing/rules`);
  const matching = rules.result.find((rule) => rule.matchers?.some((matcher) =>
    matcher.type === "literal" && matcher.field === "to" && matcher.value?.toLowerCase() === address));
  const actionMatches = (rule) => rule.actions?.length === 1 && rule.actions[0].type === "worker" &&
    rule.actions[0].value?.length === 1 && rule.actions[0].value[0] === config.name;
  if (matching) {
    if (!actionMatches(matching)) fail("An existing email rule for BANK_MAIL_ADDRESS routes elsewhere");
    if (!matching.enabled) {
      await api("PUT", `/zones/${zoneId}/email/routing/rules/${matching.id}`, {
        name: matching.name, enabled: true, actions: matching.actions, matchers: matching.matchers,
      });
    }
  } else {
    const conflicting = rules.result.find((rule) => rule.name === "Cargo bank notifications");
    if (conflicting) fail("A bank notification rule exists for a different address; rotate it deliberately");
    await api("POST", `/zones/${zoneId}/email/routing/rules`, {
      name: "Cargo bank notifications", enabled: true,
      actions: [{ type: "worker", value: [config.name] }],
      matchers: [{ type: "literal", field: "to", value: address }],
    });
  }
} finally {
  await unlink(configPath).catch(() => {});
}
console.log("Bank mail Worker deployed with its exact routing address");
