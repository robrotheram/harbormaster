Run /usr/local/bin/hermes-digest --source jobs/digest-source.json --occurrence fixture-2026-11-10.
Report the publication receipt. On failure, report the failure; never claim the note exists.
This fixed occurrence is intentional for the idempotency fixture. Production jobs must
supply a stable unique occurrence per Workbench run, reusing it for retries.
