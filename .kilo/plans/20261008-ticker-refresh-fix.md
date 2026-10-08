# Fix: 60s auto-refresh never runs after load; longpoll death leaves bot unresponsive

Two independent bugs from the 2026-10-08 incident log.

## Symptom
- 13:32 host 10.30.15.90 powered off; no new status reply appeared.
- 13:33:43 manual 🔁 button press re-pinged, worker saw `❗ ✅` (worker.go:125), replaced the reply.
- Log contains no `Tick at` lines at all — only `Tack at` (tacker, 2h longpoll recycle).

## Root cause
- `ticker = time.NewTicker(dd)` (8h) is created in the main-loop goroutine at main.go:118-120, i.e. **after** `loader()` (main.go:91).
- `loader()` → `ips.write` → `sCustomer.add` (global.go:88-102) tries `ticker.Reset(refresh)` (60s) on the 0→non-empty IPs transition, but `ticker` is still nil at load time, so the reset is skipped (global.go:95-98).
- Result: with hosts restored from pinguin.json the periodic `ips.update(customer{})` fires every 8 hours; status replies only change on manual 🔁, ⏸️, subscribe, or worker restart.

## Fix (minimal)
In main.go main-loop goroutine, arm the ticker correctly at creation based on monitored IPs:

```go
ticker = time.NewTicker(dd)
if ips.count() > 0 {
    ticker.Reset(refresh)
}
```

(Place right after `ticker = time.NewTicker(dd)`, before the loop.)

## Optional hardening (recommended, small)
On every `case t := <-ticker.C:` in main.go:129-131, after `ips.update(customer{})`, re-arm:

```go
if ips.count() > 0 {
    ticker.Reset(refresh)
} else {
    ticker.Reset(dd)
}
```

This removes reliance on the 0↔non-empty transition resets in `sCustomer.add`/`sCustomer.del` and self-heals any missed reset (they can remain; the per-tick reset makes them idempotent).

## Not a bug (no change)
- The 13:31:58 `longpoll ... context canceled` error is the old longpoll's Run() exiting after `lp.Shutdown` — expected on the 2h Tack recycle.
- The 🔁 flow itself works (worker.go:90-91, 110-133): ping → status change → delete old reply + send new one.

## Bug 2: after `longpoll ... context deadline exceeded` the bot stops responding

### Symptom
- 11:26:49 `main.go:218: longpoll longpoll-bot: Get "https://lp.vk.ru/.../wait=25": context deadline exceeded` — from then on the bot ignores messages and buttons.
- It came back to life only at 13:31:58 via the 2h `Tack` recycle (dead for ~2h5m).

### Root cause
- vksdk `longpoll-bot` `run()` (longpoll.go:241-275 in v3.3.1): any error from `check()` — including a single transient timeout (`wait=25` + 10s margin) — returns from `Run()`; there is no internal retry.
- pinguin main.go:216-220 runs `l.Run()` in a goroutine that only logs the error and exits. Nothing recreates the longpoll until the next `Tack` (2h) or manual `/restart`.
- Contributing: `l.Run()` is called without the `ttCtx` context (uses `context.Background()`), so only `Shutdown()` can stop it; the error path has no way to distinguish "shutting down on purpose" from "died unexpectedly".

### Fix
1. Extract the Tack body (main.go:132-141: `stopH(ttCancel, bh)`; recreate `ttCtx/ttCancel`; `bh, err = startH(ttCtx)`; `sendStatus(cmdRestart, err)`) into a helper, e.g. `recycleLP() (err error)`, guarded by a new `sync.Mutex` (both the main loop and a longpoll error goroutine will call it). The Tack case then calls `recycleLP()`.
2. In `startH`, replace the run goroutine with a supervised loop:
   - call `l.RunWithContext(ctx)` instead of `Run()`, where `ctx` is the `ttCtx` passed into `startH` (change the currently ignored `_ context.Context` parameter to a real one);
   - on `err == nil` or `ctx.Err() != nil` / `errors.Is(err, context.Canceled)`: log and return (deliberate stop via `stopH`);
   - otherwise: log, wait a short backoff (e.g. 3s, doubling up to 1min, reset after a successful run of ≥1min), then call `recycleLP()` and return (recycleLP spawns a new supervised longpoll; the old goroutine exits).
   - If `recycleLP()` returns an error from `startH`, keep the retry loop alive: schedule the next attempt via the same backoff (a simple way: `restart(tacker, tt)` is NOT needed; just retry `recycleLP()` inside the goroutine after backoff).
3. Keep `restart()` (manual /restart, cmdRestart) as is — it already leads to `startH` via Tack reset; optionally route it through `recycleLP()` for one code path.

### Not a bug (no change)
- The 13:31:58 `longpoll ... context canceled` line is the old longpoll's Run() exiting after `lp.Shutdown` — expected on every Tack recycle.

## Validation
1. `go vet ./...`, `go mod tidy`, `make win`.
2. Functional: with a host saved in pinguin.json, restart the binary and confirm `Tick at` lines appear in the log every ~60s.
3. Power off the monitored host; without pressing 🔁, expect a new `❗` status reply within ~1 minute and `Tick at` + worker.go:108/125 lines preceding it.
4. Unsubscribe all hosts (…❌) and confirm the ticker falls back to `dd` (no `Tick at` spam in idle logs).
5. Longpoll resilience: temporarily block egress to `lp.vk.ru` (firewall) for ~40s while the bot runs → expect `context deadline exceeded` in the log followed by a reconnect attempt within seconds and the bot answering messages again once connectivity returns; the backoff must not spin (bounded at ~1min).
6. Tack recycle (wait 2h or temporarily shorten `tt` for the test): the old longpoll still logs `context canceled`, a new one starts, exactly one longpoll goroutine remains; no mutex deadlock with the error-path `recycleLP()`.
