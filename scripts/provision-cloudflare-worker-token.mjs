// Creates a scoped Cloudflare account token and stores its value in GitHub.
// The token value stays in memory and is never printed.
import { spawnSync } from "node:child_process";

const repository = "mleczakm/cargo.mleczki.pl";
const environment = "production";
const domain = "mleczki.pl";
const tokenName = "cargo-worker-deploy";

function run(program, args, input) {
  const result = spawnSync(program, args, { encoding: "utf8", input, maxBuffer: 1024 * 1024 });
  if (result.status !== 0) throw new Error(`${program} ${args[0]} failed: ${result.stderr.trim()}`);
  return result.stdout;
}

function cf(args) {
  return JSON.parse(run("cf", args));
}

const zone = cf(["zones", "get", "--zone", domain]);
if (zone.name !== domain || !zone.account?.id) throw new Error("Cloudflare zone/account mismatch");
const existing = cf(["accounts", "tokens", "list"]);
if (existing.some((token) => token.name === tokenName && token.status === "active")) {
  throw new Error(`${tokenName} already exists; inspect it before creating another token`);
}
const groups = cf(["accounts", "tokens", "permission-groups", "list"]);
function ids(names) {
  return names.map((name) => {
    const group = groups.find((item) => item.name === name && item.is_selectable);
    if (!group) throw new Error(`Permission unavailable: ${name}`);
    return { id: group.id };
  });
}
const policies = [
  {
    effect: "allow",
    resources: { [`com.cloudflare.api.account.${zone.account.id}`]: "*" },
    permission_groups: ids(["Workers Scripts Write", "Queues Write"]),
  },
  {
    effect: "allow",
    resources: { [`com.cloudflare.api.account.zone.${zone.id}`]: "*" },
    permission_groups: ids(["Zone Read", "Zone Settings Write", "Email Routing Rules Read", "Email Routing Rules Write"]),
  },
];
if (process.argv.includes("--dry-run")) {
  run("cf", ["accounts", "tokens", "create", "--name", tokenName, "--policies", JSON.stringify(policies), "--dry-run"]);
  console.log("Token policy validated locally");
  process.exit(0);
}

const token = cf(["accounts", "tokens", "create", "--name", tokenName, "--policies", JSON.stringify(policies)]);
if (!token.value || !token.id) throw new Error("Cloudflare did not return a token value");
const verify = await fetch(`https://api.cloudflare.com/client/v4/accounts/${zone.account.id}/tokens/verify`, {
  headers: { Authorization: `Bearer ${token.value}` },
});
const verified = await verify.json();
if (!verify.ok || !verified.success || verified.result?.id !== token.id || verified.result?.status !== "active") {
  throw new Error("New Cloudflare token did not verify; token ID: " + token.id);
}
run("gh", ["secret", "set", "CLOUDFLARE_WORKER_TOKEN", "-R", repository, "--env", environment], token.value);
console.log(`Stored active Cloudflare token ${token.id} as ${environment}/CLOUDFLARE_WORKER_TOKEN`);
