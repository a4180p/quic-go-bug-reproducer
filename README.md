# quic-go CloseIdleConnections reproducer

**The test is expected to fail on affected quic-go versions.** It asserts the documented behavior; it does not change that assertion to accept the bug.

## Run

```sh
go test -count=1
```

## What it does

1. Start a local HTTP/3 server. The standard-library httptest server supplies only the test TLS certificate.
2. Send HTTP 200 and `Content-Length: 11`; write and flush `first`.
3. Wait on a channel before sending `second`.
4. The client reads exactly `first`, so its response body is still active.
5. In `with_cleanup`, call `http3.Transport.CloseIdleConnections()`. The control omits only this call.
6. Release the server and read the rest of the body. Both cases should return `firstsecond` without error.

QUIC idle limits on both peers and the HTTP/3 server idle limit are one hour. A five-second client timeout only bounds a hung test. There are no sleeps or periodic cleanup timers.

## Verified results

2026-11-03, `go1.27.3 darwin/arm64`:

| quic-go | Without cleanup | With cleanup | Race reports |
| --- | --- | --- | --- |
| v0.65.0 | 10/10 pass | 10/10 fail | 0 |

Observed output:

```text
cleanup=false status=200 body="firstsecond" error=<nil>
cleanup=true status=200 body="first" error=H3 error (0x0) (local)
active response interrupted: got "first", want "firstsecond"; read error: H3 error (0x0) (local)
```
