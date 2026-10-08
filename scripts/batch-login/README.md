# Batch login (CloakBrowser)

Logs into a list of our own crawler accounts via username/password, one
CloakBrowser instance per account (optionally through that account's own
proxy), and saves each resulting session to `sessions/<username>.json` so
it never has to log in by hand again -- the crawler can just load that
file into `browser.new_context(storage_state=...)`.

Accounts that already have a saved session are skipped by default (see
`SKIP_IF_SESSION_EXISTS` in `.env`), so re-running this is safe and only
logs in whatever is new/expired.

## Setup

```sh
pip install -r requirements.txt
cp .env.example .env               # set LOGIN_URL + the form's CSS selectors
cp accounts.csv.example accounts.csv   # username,password,proxy (proxy optional)
```

Figure out the selectors once, headed, against a single account before
running the full batch:

```sh
# in .env: PLAYWRIGHT_HEADLESS=false, CONCURRENCY=1
python batch_login.py
```

Then switch back to `PLAYWRIGHT_HEADLESS=true` and raise `CONCURRENCY` to
something your proxy pool / the target site's rate limits can sustain
(100 accounts does not mean 100 browsers at once -- start small, e.g. 5).

### Smoke-testing the mechanics (optional)

`.env.microsoft-smoketest.example` is a worked example against
`login.microsoftonline.com`'s real (two-step: email -> Next -> password)
form, for verifying the fill/click/wait-for-success/save-session mechanics
work at all before pointing the script at your real target. Use it with
exactly **one** account you own and `accounts.csv` containing a single
row -- it's a mechanics check, not a login target. Swap back to your real
site's `LOGIN_URL`/selectors (`.env.example`) once it passes.

## Run

```sh
python batch_login.py            # reads ./accounts.csv
python batch_login.py other.csv  # or an explicit path
```

Output:
- `sessions/<username>.json` -- one storage_state per successfully logged-in account
- `batch_login_output.json` -- ok/fail summary per account (no credentials in it)

## Notes

- `accounts.csv`, `sessions/`, `.env`, and the output summary are
  gitignored -- they contain credentials/session cookies and must never
  be committed.
- If the target site puts a captcha or OTP step in front of the
  password form, this script can't click through that on its own --
  run headed for those accounts and finish the step by hand once, then
  rely on the saved session afterwards.
