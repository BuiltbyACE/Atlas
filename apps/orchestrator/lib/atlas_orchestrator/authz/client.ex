defmodule AtlasOrchestrator.Authz.Client do
  @moduledoc """
  The contract for asking Go whether a user holds a permission
  (docs/ATLAS.md §7.5).

  Go is the authority on roles→permissions (ADR-008); the orchestrator never
  learns the mapping. A client answers a single question — "does this user hold
  this permission?" — and reports transport faults as `{:error, reason}` so the
  caller can fail closed. `AtlasOrchestrator.Authz.HTTP` is the production
  implementation; tests inject a stub.

  A client is any value `AtlasOrchestrator.Authz` can call: a module exporting
  `check/2`, or a 2-arity function (convenient in tests).
  """

  @callback check(user_id :: String.t(), permission :: String.t()) ::
              {:ok, boolean()} | {:error, term()}
end
