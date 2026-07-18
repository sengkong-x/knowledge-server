# Adopt `fsnotify` and `goldmark` as dependencies

Until now the project has depended only on `yaml.v3`. Ticket 06 (Productivity Experience) needs two new capabilities the standard library doesn't provide well: detecting Vault filesystem changes, and rendering Markdown to HTML. For the watcher, we considered a stdlib polling loop (periodic `os.Stat`/walk, diffing mtimes) against `fsnotify`; we chose `fsnotify` because it's event-driven (no polling latency or wasted CPU), and it correctly handles OS-level edge cases — like editors that save via write-to-temp-then-rename — that a hand-rolled poller would get wrong on day one. For Markdown rendering, we chose `goldmark` over a hand-rolled parser or a client-side JS renderer; it was already named in the original spec's target architecture, and rendering server-side keeps the "renderer is dumb, the engine does the work" principle intact — no second Markdown dialect/implementation living in the browser.

## Consequences

Both are widely-used, actively maintained libraries (`fsnotify` backs Kubernetes and Prometheus's file watching; `goldmark` is the Markdown engine Hugo uses), so the lock-in risk is low. This ends the project's "stdlib + yaml only" era — a future contributor should expect the dependency list to stay small and deliberate, not treat this as license to add dependencies freely.
