# Use stdlib net/http with no router dependency

The project's minimal-deployment principle calls for a single binary with no required runtime dependencies. We decided to build the HTTP layer entirely on stdlib `net/http.ServeMux`, relying on Go 1.22+'s enhanced method- and wildcard-aware routing, rather than adding a router library such as chi or gorilla/mux. This is a deliberate choice, not an oversight — a future contributor should not "fix" this by introducing a router dependency.
