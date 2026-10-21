# Changelog

> Generated from `git` by `scripts/gen-changelog.sh` — do not edit by hand.

- **v1.0.5** (2026-10-20) · `fix` — zen free tier: force stream:true for one-shot calls (gate rejects stream:false, broke title generation), add provider diagnostics logging, default title model to GLM Flash Free
- **v1.0.4** (2026-10-20) · `fix` — add tests for message/session/db/fileutil/version/provider and fix 3 bugs they revealed (image parts dropped on read, glob limit erroring, nil config on debug path)
- **v1.0.3** (2026-10-20) · `fix` — fix Zen provider free tier: correct base URL to zen/v1, add client fingerprint (session, UA, required tools), refresh free model catalog
- **v1.0.2** (2026-10-20) · `fix` — clean --version output (no stray warnings, plain X.Y.Z) and English-only user-facing text
- **v1.0.1** (2026-10-20) · `fix` — release workflow: accept a tag via workflow_dispatch input and resolve it for both triggers
- **v1.0.0** (2026-10-20) · `major` — first release of the opencode Go fork (TS v2 parity)
