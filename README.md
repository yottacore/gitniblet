Storage branch for CLA signatures.

The CLA workflow (`.github/workflows/cla.yml`) commits contributor signatures here as `signatures/cla.json`. It is not part of the source tree and never merges into `dev` or `main`. Leave this branch unprotected - the workflow has to push to it.
