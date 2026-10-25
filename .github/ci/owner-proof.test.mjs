import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { jobRuns } from './workflows.mjs';

const read = name => readFileSync(new URL(`../workflows/${name}.yml`, import.meta.url), 'utf8').replaceAll('\r\n', '\n');
const entry = read('native-service-contracts');
const proof = read('linux-owner-proof');
// Focused source guard, including reusable jobs (which have uses, not steps).
function job(source, name) {
  const match = source.match(new RegExp(`^  ${name}:\\n([\\s\\S]*?)(?=^  [\\w-]+:|$(?![\\s\\S]))`, 'm'));
  assert.ok(match, `missing job ${name}`);
  const condition = match[1].match(/^    if: (.+)$/m);
  assert.ok(condition, `missing condition ${name}`);
  return { if: condition[1], text: match[1] };
}

test('owner proof dispatch excludes native matrix and preserves ordinary entry points', () => {
  assert.match(entry, /      owner_proof:\n        description: .+\n        type: boolean\n        default: false\n/);
  const owner = job(entry, 'owner-proof');
  const services = job(entry, 'services');
  assert.match(owner.text, /^    uses: \.\/\.github\/workflows\/linux-owner-proof.yml$/m);
  assert.doesNotMatch(owner.text, /^    (steps|strategy|runs-on):/m);
  for (const event of ['pull_request', 'push', 'workflow_dispatch']) {
    for (const value of [undefined, false, true]) {
      for (const runner of ['all', 'ubuntu-24.04', 'windows-latest']) {
        for (const faults_only of [false, true]) {
          const context = { github: { event_name: event }, inputs: { owner_proof: value, runner, faults_only }, functions: { success: () => true } };
          const pilot = event === 'workflow_dispatch' && value === true;
          assert.equal(jobRuns(owner, context), pilot, JSON.stringify(context));
          assert.equal(jobRuns(services, context), !pilot, JSON.stringify(context));
        }
      }
    }
  }
  assert.match(proof, /^on:\n  workflow_call:\n/m);
  assert.doesNotMatch(proof, /^  (workflow_dispatch|push|pull_request):/m);
  assert.equal((proof.match(/^          - runner:/gm) ?? []).length, 2);
  assert.match(proof, /max-parallel: 2/);
  assert.match(proof, /-test.timeout=60s/);
  assert.match(proof, /UNMET: actual Linux 5.8/);
  assert.match(proof, /UNMET: complete validated app identity/);
  assert.match(proof, /          exit 1/);
});

test('proof uses the repository action pins, never mutable action tags', () => {
  const pins = new Map([
    ['checkout', '3d3c42e5aac5ba805825da76410c181273ba90b1'],
    ['setup-go', 'b7ad1dad31e06c5925ef5d2fc7ad053ef454303e'],
    ['upload-artifact', 'cf430e030ddbb5b0abf93d22962f4752f3646cd9'],
    ['download-artifact', '9000827ccba6bdab643e8b6fd33ac0654aef8333'],
  ]);
  const uses = [...proof.matchAll(/uses: actions\/([\w-]+)@(\S+)/g)];
  assert.equal(uses.length, 6);
  for (const [, action, revision] of uses) assert.equal(revision, pins.get(action));
});
