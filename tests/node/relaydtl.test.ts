import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";
import assert from "node:assert/strict";

type Balance = {
  available?: number;
  held?: number;
};

type ScenarioResult = {
  ok: boolean;
  error?: string;
  steps: Array<{
    op: string;
    ok: boolean;
    error_code?: string;
    receipt?: { id: string; route_id: string; fee: number };
    settlement?: {
      receipt_id: string;
      principal: number;
      fee: number;
      released_refund: number;
      charge_source: { from_held: number; from_reserve: number };
    };
    view?: {
      delivered_weight: number;
      rejected_weight: number;
      receipts: Record<string, number>;
    };
  }>;
  snapshot: {
    balances: Record<string, Record<string, Balance>>;
    messages: Array<{ id: string; status: string }>;
    receipts: Array<{ id: string; status: string; route_id: string }>;
    totals: Record<string, number>;
  };
};

type Scenario = Record<string, unknown>;

const repo = resolve(fileURLToPath(new URL("../..", import.meta.url)));
const goBin = process.env.GO ?? "go";

function runScenario(scenario: Scenario): ScenarioResult {
  const dir = mkdtempSync(join(tmpdir(), "relaydtl-"));
  const path = join(dir, "scenario.json");
  writeFileSync(path, JSON.stringify(scenario, null, 2));
  const run = spawnSync(goBin, ["run", "./cmd/relaydtl", "--scenario", path, "--pretty=false"], {
    cwd: repo,
    encoding: "utf8",
  });
  if (run.error) {
    throw run.error;
  }
  assert.equal(run.status, 0, run.stderr || run.stdout);
  const parsed = JSON.parse(run.stdout) as ScenarioResult;
  assert.equal(parsed.ok, true, parsed.error);
  return parsed;
}

function baseScenario(overrides: Partial<Scenario> = {}): Scenario {
  return {
    name: "base",
    now: 10,
    policy: {
      quorum_weight: 2,
      reject_weight: 2,
      max_message_ttl: 120,
      max_receipt_drift: 20,
      reserve_account: "relay:reserve",
    },
    accounts: [
      { id: "payer:alice", kind: "user", balances: { USDC: 10_000 } },
      { id: "beneficiary:bob", kind: "user", balances: { USDC: 0 } },
      { id: "operator:west", kind: "operator", balances: { USDC: 0 } },
      { id: "operator:east", kind: "operator", balances: { USDC: 0 } },
      { id: "relay:reserve", kind: "reserve", balances: { USDC: 50_000 } },
    ],
    nodes: [
      { id: "node:a", weight: 1, operator: "operator:west" },
      { id: "node:b", weight: 1, operator: "operator:west" },
      { id: "node:c", weight: 1, operator: "operator:east" },
    ],
    routes: [
      {
        id: "route:west",
        source: "node:a",
        target: "node:c",
        asset: "USDC",
        operator: "operator:west",
        capacity: 5_000,
        fee_bps: 20,
        latency: 4,
        priority: 1,
      },
      {
        id: "route:east",
        source: "node:a",
        target: "node:c",
        asset: "USDC",
        operator: "operator:east",
        capacity: 5_000,
        base_fee: 2,
        fee_bps: 25,
        latency: 6,
        priority: 2,
      },
    ],
    steps: [],
    ...overrides,
  };
}

function balance(result: ScenarioResult, account: string, asset: string): Balance {
  return result.snapshot.balances[account]?.[asset] ?? {};
}

test("settles a delivered message after weighted confirmations", () => {
  const result = runScenario(
    baseScenario({
      name: "settlement",
      steps: [
        {
          op: "submit_message",
          message_id: "msg:settle",
          source: "node:a",
          target: "node:c",
          payer: "payer:alice",
          beneficiary: "beneficiary:bob",
          asset: "USDC",
          amount: 1_000,
          fee_limit: 25,
          expires_at: 80,
        },
        { op: "receipt", message_id: "msg:settle", receipt_id: "rcpt:settle", route_id: "route:west" },
        { op: "confirm", receipt_id: "rcpt:settle", node_id: "node:a" },
        { op: "confirm", receipt_id: "rcpt:settle", node_id: "node:b" },
        { op: "settle", receipt_id: "rcpt:settle" },
      ],
    }),
  );

  assert.equal(balance(result, "beneficiary:bob", "USDC").available, 1_000);
  assert.equal(balance(result, "operator:west", "USDC").available, 2);
  assert.equal(balance(result, "payer:alice", "USDC").available, 8_998);
  assert.equal(balance(result, "payer:alice", "USDC").held ?? 0, 0);
  assert.equal(result.snapshot.totals.USDC, 60_000);
});

test("keeps settlement pending until quorum is complete", () => {
  const result = runScenario(
    baseScenario({
      name: "partial-confirmations",
      steps: [
        {
          op: "submit_message",
          message_id: "msg:partial",
          source: "node:a",
          target: "node:c",
          payer: "payer:alice",
          beneficiary: "beneficiary:bob",
          asset: "USDC",
          amount: 900,
          fee_limit: 20,
          expires_at: 70,
        },
        { op: "receipt", message_id: "msg:partial", receipt_id: "rcpt:partial", route_id: "route:west" },
        { op: "confirm", receipt_id: "rcpt:partial", node_id: "node:a" },
        { op: "settle", receipt_id: "rcpt:partial", expect_error: true },
        { op: "view", message_id: "msg:partial" },
        { op: "confirm", receipt_id: "rcpt:partial", node_id: "node:b" },
        { op: "settle", receipt_id: "rcpt:partial" },
      ],
    }),
  );

  assert.equal(result.steps[3].ok, true);
  assert.equal(result.steps[3].error_code, "not_ready");
  assert.equal(result.steps[4].view?.delivered_weight, 1);
  assert.equal(balance(result, "beneficiary:bob", "USDC").available, 900);
});

test("expires messages that do not receive a timely receipt", () => {
  const result = runScenario(
    baseScenario({
      name: "expiration",
      steps: [
        {
          op: "submit_message",
          message_id: "msg:late",
          source: "node:a",
          target: "node:c",
          payer: "payer:alice",
          beneficiary: "beneficiary:bob",
          asset: "USDC",
          amount: 700,
          fee_limit: 18,
          expires_at: 30,
        },
        { op: "advance", to: 35 },
        { op: "receipt", message_id: "msg:late", receipt_id: "rcpt:late", route_id: "route:west", expect_error: true },
        { op: "expire", message_id: "msg:late" },
      ],
    }),
  );

  assert.equal(balance(result, "payer:alice", "USDC").available, 10_000);
  assert.equal(balance(result, "payer:alice", "USDC").held ?? 0, 0);
  assert.equal(result.snapshot.messages.find((message) => message.id === "msg:late")?.status, "expired");
});

test("selects an alternative route when the primary route lacks capacity", () => {
  const result = runScenario(
    baseScenario({
      name: "alternative-route",
      routes: [
        {
          id: "route:west",
          source: "node:a",
          target: "node:c",
          asset: "USDC",
          operator: "operator:west",
          capacity: 500,
          fee_bps: 20,
          latency: 4,
          priority: 1,
        },
        {
          id: "route:east",
          source: "node:a",
          target: "node:c",
          asset: "USDC",
          operator: "operator:east",
          capacity: 5_000,
          base_fee: 2,
          fee_bps: 25,
          latency: 6,
          priority: 2,
        },
      ],
      steps: [
        {
          op: "submit_message",
          message_id: "msg:alt",
          source: "node:a",
          target: "node:c",
          payer: "payer:alice",
          beneficiary: "beneficiary:bob",
          asset: "USDC",
          amount: 1_200,
          fee_limit: 30,
          expires_at: 80,
        },
        { op: "receipt", message_id: "msg:alt", receipt_id: "rcpt:alt" },
        { op: "confirm", receipt_id: "rcpt:alt", node_id: "node:a" },
        { op: "confirm", receipt_id: "rcpt:alt", node_id: "node:c" },
        { op: "settle", receipt_id: "rcpt:alt" },
      ],
    }),
  );

  assert.equal(result.steps[1].receipt?.route_id, "route:east");
  assert.equal(balance(result, "beneficiary:bob", "USDC").available, 1_200);
  assert.equal(balance(result, "operator:east", "USDC").available, 5);
});
