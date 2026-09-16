# Go API Automation Tests — Mock Borrow API

Black-box API automation tests for [mock-borrow-api](https://github.com/kennethchuaqiyang/mock-borrow-api), written in Go using only the standard library (`net/http` + `testing`) — no external test framework needed.

**Target:** https://mock-borrow-api.onrender.com (live deployed service)

Unlike the server repo's own `main_test.go` (which calls handler functions directly, in-process), these are true end-to-end tests: real HTTP requests over the network against the deployed service — the same way any external API automation suite would test it.

## Coverage

**GET /api/user**
- Happy path — correct metadata/cache for a known user
- Response headers — `X-Browser` echo, `X-Secret-Key` format
- Missing required params → 400
- Non-integer `userid` → 400
- Wrong method (POST) → 405

**POST /api/borrow**
- Happy path — borrow within limit, `amount_owed` updates correctly
- Over limit — rejected, `amount_owed` unchanged
- Exactly at the 200 limit — still allowed (only "above" 200 is rejected)
- Missing username → 400
- Invalid JSON body → 400
- Wrong method (GET) → 405
- Response headers — `X-Browser` echo, `X-Secret-Key` format

Each borrow test uses a randomized `userid` so tests don't interfere with each other's balances when run against the shared live database.

## Running

```bash
go test -v .
```

## Notes

- The target service runs on Render's free tier, which spins down after inactivity — the first request in a run may take 20-30 seconds while it wakes up. The `http.Client` timeout is set to 40 seconds to allow for this.
- Data is stored in a real Postgres database (Neon), so borrow amounts persist across test runs — this is why each test creates a fresh random user rather than reusing a fixed one.
- To point these tests at a different environment (e.g. localhost during development), change the `baseURL` constant in `api_test.go`.