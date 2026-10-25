# Linux attribution foundations: review handoff

25 October 2026. **Draft, not a working product backend or release proof.**
The engine still uses its existing scanner, UDP generation/destroy checks,
one-second reconciliation and both forwarding paths unchanged.

| Obligation | This change | Evidence now |
| --- | --- | --- |
| Original ABI and probes | Cookie initialization, IPv4 UDP attempt evidence, IPv4 packet tuple collision latch | Pilot e112: generation twice byte-identical; loaded on modern x64/arm64 |
| Owned loader | Direct cgroup links and one PMU FD; no tracefs/legacy fallback or pins | Pilot exposed duplicate callbacks; corrected attachment awaits next kernel run |
| Safety decision | No known-owner result; ambiguity/poison/incomplete identity cannot fall back | Four pure tests passed on Windows |
| Connected UDP actor, exit/exec | Inherited connected FD; two exec generations retained after exit | Pilot e112 retained evidence on both architectures; complete identity still 0/2 |
| Queued tuple reuse | Two sockets, same port and bytes, both packets queued before any TUN read | ARM64 pilot passed; x64 encountered an IPv6 router solicitation; IPv4-only fixture fix awaits rerun |
| Linux 5.8, both architectures | No guest infrastructure found in existing workflows/scripts | **Unmet**; modern namespaces do not qualify the floor |
| Complete identity | PID/start/exec/device/inode evidence is deliberately insufficient | **Unmet**, complete identity numerator remains zero |

First review follow-up: all 106 CI tooling tests passed (zero skips), including
the owner-proof dispatch exclusivity and action-pin guards. Four pure Go tests,
tagged Linux amd64/arm64 cross-compilation and Linux amd64 vet also passed.
These checks did not run the Linux fixtures or dispatch the hosted pilot.

## First hosted pilot and diagnostic fixes

[Run 40182049682](https://github.com/kekawia/net-switch/actions/runs/40182049682)
used e112b1929c75957301b587c66b594d4877b010c2 on kernel 6.17.0-1022-azure,
x86_64 and aarch64. The parent-dispatched builder succeeded with two-pass
byte equality. ARM64 demonstrated cookies 4097/4098 and identical queued
packets while B remained live. Both jobs still failed; this is not qualification.

- One oversized send advanced Attempts from 24 to 28, with Packets unchanged
  at 6. The loader created one local kprobe per online CPU. Linux 5.8 and 6.17
  run the BPF program before checking the per-CPU perf-event list, so these
  were four real callbacks, not a counter-format issue. The fix owns one PMU
  FD with pid=-1/cpu=0, matching the pinned loader's mechanism. Counts remain
  exact. A new bounded fixture pins a thread to each online CPU, checks its
  actual CPU around the send, requires exactly one attempt/packet and validates
  received bytes. Missing CPU access or a changed online set fails coverage.
- Cleanup reached wrapped ENOENT after proving the objects existed. The
  SDK wraps both ID lookup errors with `%w`; `os.IsNotExist` missed that chain.
  The cleanup check now uses `errors.Is(err, unix.ENOENT)`. Every other error
  still fails, and the fresh coexisting traffic proof remains required.
- x64 received an IPv6 router solicitation before the expected packets. Only
  the new private TUN's `disable_ipv6` is set and read back before bringing it
  up. No host/default sysctl changes or packet-discard loop are used.

These diagnostic fixes preserve the parent's `ns_owner_license` rename and
ELF license section. They are not credited as kernel passes until rerun.
Local validation of the fixes: four pure tests passed, the complete tagged
fixture cross-compiled for Linux amd64/arm64, and Linux amd64 vet passed. No
Linux execution, pilot dispatch or CI watcher was started by the worker.

## Must resolve before integration

**The tuple bridge loses packet generation.**

Now: A sends datagram P using cookie 41, closes, and P remains in the TUN queue.
B reuses its tuple, sends identical P using cookie 42, and remains live. A latest
tuple lookup returns 42 when the engine finally reads A's packet. Keeping both
cookies exposes ambiguity but cannot tell which generated that packet. Waiting
one second, or any fixed TTL, adds no evidence of consumption. Counting egress
events does not establish TUN delivery: packets can be dropped after that hook.

Should: attribution must refer to the generation of the packet being forwarded.

Fix: the smallest safe behavior is unknown for overlapping generations, with
no holder-scan override and no time-based reclamation. This prototype retains
all 256 tuple slots and poisons the entire collection on insertion failure.
It therefore bounds memory but eventually loses completeness under churn.
To reclaim safely and still meet the rate gates requires a separately proven
drain/acknowledgement boundary or a capture interface that delivers a generation
token together with each packet. That is a design change, not another tuple TTL.
Dependent Owners/forwarding integration stops here.

The kernel fixture uses bound, unconnected UDP sockets with DF and Sendto.
It proves A is closed before reusing its port, retains B through both TUN reads,
and checks B's SO_COOKIE before and after draining. It demands distinct cookies,
the exact tuple, DF/zero IP ID, payload and byte-identical captured packets;
it reads the collision before draining either packet. No captured byte is edited.
A failure of any precondition fails the fixture. The pure model is not credited
as a successful real-kernel reproduction.

**Executable evidence is not a complete app identity.**

Now: the probe retains group-leader boot-time start, exec counter and executable
device/inode after the process exits. It records send attempts, including failed
attempts; it never calls them accepted data or a first-sender decision. It does
not validate paths, mount/sandbox metadata, AppImage registrations or app equality.

Should: complete trusted app identity must survive exit/exec and distinguish
different apps from different processes of the same app.

Fix: a retained, validated identity mechanism is still required. A later read
of the current PID's metadata cannot fill old evidence safely. The test reports
0/2 complete identities, even when both sender-to-cookie joins work. No
notification reader exists, so notifications cannot accidentally hide this gap.

## Independent foundations

Original `bpf/abi.h` and `bpf/attribution.c` use fixed-width records and minimal
CO-RE field projections. No upstream program, helper header or path walker was
copied. Only those two BPF files are offered under **MIT OR GPL-2.0-only**;
their ELF license is `Dual MIT/GPL`. The MIT option uses the root MIT grant.
Userspace and the root license remain unchanged. This is the deliberate BPF
licensing choice from the reviewed implementation handoff.

The deliberately reused dependency is cilium/ebpf **v0.22.0**, MIT, including
its unmodified vendored license. The Go tool directive pins bpf2go to the same
module. Existing x/sys v0.49.0 and all other dependency versions are retained.
`go mod vendor` generated the new vendor tree; it was not edited by hand.

The generator pins Go 1.27.3 and Ubuntu's clang 18.1.3-1 compiler/libclang/LLVM
package bytes by SHA-256. It uses only committed type declarations, `-nostdinc`,
`-mcpu=v1`, canonical `/ns-owner` debug/source paths and fixed locale/timezone.
It builds both architecture-specific objects twice and compares every output.
It does not execute on Windows. The host's libc/runtime is still a builder
prerequisite, not a pinned complete container image.

**Generated objects under gitignored `dist/ownerbpf` are proof artifacts only.**
The e112 pilot generated reproducible objects and loaded them on both modern
architectures; they remain uncommitted proof artifacts with no embedded production
object. The future tracked production destination is
`internal/engine/linux/ownerbpf/generated/`. Producing objects there and adding
a CI regeneration-versus-committed-output diff check remain explicitly pending
before integration; neither is implemented by this proof-artifact generator.
Modern verifier acceptance does not establish the Linux 5.8 floor.

The loader requests the actual three programs/maps/helper combinations. It
uses `AttachRawLink`, not the high-level cgroup fallback. Kprobes use the PMU
`perf_event_open` ABI directly, one system-wide event (pid=-1/cpu=0).
The CPU coverage fixture is bounded to 512 CPUs and ten seconds, reusing one
socket so its actor/tuple occupancy does not grow with CPU count. The loader
owns one PMU FD and two cgroup link FDs per collection; partial-load and crash
fixtures assert that PMU ownership count as well as actual object pre-existence.
Every acquired FD is owned and unwound in reverse order on errors. No other
application's attachment is replaced or detached. The opt-in crash fixture
first proves its links/programs exist, kills the owner, checks their removal,
and proves the original collection still observes a fresh datagram with checked
payload/source and advancing Attempts/Packets, while the child's objects remain
gone. Partial-attachment faults ran in the pilot but their absence checks failed
on the wrapped ENOENT bug; corrected cleanup evidence remains pending.

Maps have 256 immutable actor records (32-byte key, 32-byte value), 256 tuple
records (12-byte key, 16-byte value), seven 64-bit counters and one 64-bit scope
entry. Hash key/value plus array-value payload totals **23,616 bytes**, excluding hash buckets,
alignment, allocator metadata, programs/BTF and perf events. There is no LRU,
history deletion, temporary send map, metadata-string store or event queue.
Kernel allocations and Go heap/RSS are **not measured**. The fixture's 32 MiB
memlock ceiling is a test limit, not an adopted product memory allowance.
cilium's verifier-log allocation may grow above its 64 KiB starting size;
this is not a 64 KiB heap bound. Error messages identify the operation and
retain errno plus the library's concise verifier summary.

The tuple key intentionally has no namespace token: the fixture runs in one
private network namespace and one dedicated cgroup leaf. This is not suitable
for a host-wide backend. TCP/Fast Open, IPv6, sendpage/corking, worker provenance,
fork lifecycle tracking, complete identities, app conflicts, packet-bound
generation, safe reclamation, overhead and actual floor qualification remain
unimplemented or unproven. No release blocker is removed.

## Parent's next commands

Review the reusable workflow first. Its registered entry point is
`native-service-contracts.yml` with `owner_proof=true`; that selection excludes
the ordinary five-runner native matrix. PR/push/default dispatch behavior stays
unchanged. The proof has four jobs total, only two concurrent
kernel fixtures, a 60-second fixture timeout and no acceptance/stress dispatch.
Its final qualification job deliberately fails on the known unmet obligations.

```sh
gh workflow run native-service-contracts.yml --repo kekawia/net-switch --ref diagnostic/linux-owner-pilot -f owner_proof=true
```

Diagnostic fixes are pushed only to `diagnostic/linux-owner-pilot`; the parent
promotes the reviewed final head to PR100. The worker does not dispatch or watch
the pilot. On the hosted builder, the exact
generation command is `node scripts/generate-owner-bpf.mjs`. Each modern runner
builds the tagged fixture and runs it in `unshare --net`; no engine or service
is started. Actual 5.8 guests need a separate reviewed infrastructure plan.
Pass `NS_OWNERBPF_KERNEL=<exact uname -r>` inside a guest to reject a wrong kernel.

Before this can become a qualified foundations PR: inspect compiler output,
fix any verifier/fixture failures, obtain and commit the reproducible objects,
fault-inject partial attachment, measure allocations/overhead, and run actual
5.8 x64/arm64. Do not treat this draft's cross-compilation as those results.

## Source decisions checked

- [Linux 5.8 kprobe BPF ordering](https://github.com/torvalds/linux/blob/v5.8/kernel/trace/trace_kprobe.c),
  [Linux 6.17 ordering](https://github.com/torvalds/linux/blob/v6.17/kernel/trace/trace_kprobe.c)
  and [PMU event creation](https://github.com/torvalds/linux/blob/v6.17/kernel/trace/trace_event_perf.c):
  each PMU event creates a local probe; `trace_call_bpf` runs before the per-CPU
  perf-event list is checked. The single-FD setup follows
  [cilium v0.22.0 pmuProbe](https://github.com/cilium/ebpf/blob/v0.22.0/link/kprobe.go).

- [Linux 5.8 IP-ID selection](https://github.com/torvalds/linux/blob/v5.8/include/net/ip.h)
  and [Linux 6.12 IP-ID selection](https://github.com/torvalds/linux/blob/v6.12/include/net/ip.h):
  connected sockets use a per-socket ID even with DF. The unconnected DF branch
  sets ID zero, allowing real byte equality without altering captured data.

- [Linux 5.8 UDP](https://github.com/torvalds/linux/blob/v5.8/net/ipv4/udp.c):
  the cgroup sendmsg hook excludes connected sends; pending sendpage bypasses
  udp_sendmsg. This prototype deliberately covers only the latter entry symbol.
- [Linux 5.8 IPv6 UDP](https://github.com/torvalds/linux/blob/v5.8/net/ipv6/udp.c):
  the connected-path exclusion also exists there; IPv4 proof is not IPv6 proof.
- [Linux 5.8 TCP output](https://github.com/torvalds/linux/blob/v5.8/net/ipv4/tcp_output.c):
  TCP_CONNECT_CB precedes SYN/Fast Open transmission, but is not implemented here.
- [Linux 5.8 helpers](https://github.com/torvalds/linux/blob/v5.8/net/core/filter.c),
  [task fields](https://github.com/torvalds/linux/blob/v5.8/include/linux/sched.h)
  and [socket fields](https://github.com/torvalds/linux/blob/v5.8/include/net/sock.h):
  cookie creation uses the cgroup context helper; the tracing program reads the
  initialized field. It does not call the cookie helper on an arbitrary sock.
- [Linux 5.8 TUN serialization](https://github.com/torvalds/linux/blob/v5.8/drivers/net/tun.c)
  and [egress ordering](https://github.com/torvalds/linux/blob/v5.8/net/ipv4/ip_output.c):
  ordinary TUN packet bytes/virtio headers do not carry a socket cookie; egress
  observation is earlier than final device delivery.
- [cilium v0.22.0 cgroup attachment](https://github.com/cilium/ebpf/blob/v0.22.0/link/cgroup.go)
  and [kprobe attachment](https://github.com/cilium/ebpf/blob/v0.22.0/link/kprobe.go):
  both high-level helpers can select lifetime mechanisms this task disallows.
- [Kernel licensing documentation](https://docs.kernel.org/bpf/bpf_licensing.html)
  and [5.8 recognized license strings](https://github.com/torvalds/linux/blob/v5.8/include/linux/license.h)
  support the chosen declaration; [versioned bpf2go](https://github.com/cilium/ebpf/blob/v0.22.0/cmd/bpf2go/main.go)
  defines the actual generation flags.
