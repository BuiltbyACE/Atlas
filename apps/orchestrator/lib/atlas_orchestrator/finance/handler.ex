defmodule AtlasOrchestrator.Finance.Handler do
  @moduledoc """
  Handles finance events from the outbox (§7.4) and pushes realtime signals
  (§7.5, ADR-009).

  Go owns finance state; this handler only nudges the views that changed:

    * `finance:board` — the finance board (expenses, invoices, payments,
      budgets), for anyone who holds `finance.expense.read`. Joining is
      authorised by Go's internal join check
      (`AtlasOrchestrator.Realtime.FinanceChannel` → `AtlasOrchestrator.Authz`).
    * `exec:dashboard` — a `dashboard.changed` tile invalidation on every event,
      because each one writes an audit row and moves the cash, receivables and
      budget tiles.

  Payloads are identifiers and change type only; clients refetch from Go, which
  enforces permissions. Finance events name a claimant (`claimant_person_id`)
  and an actor (`actor_id`) — persons, not account ids — so no personal nudge is
  sent: a socket is addressed by an account id (§7.5, §10.1), and Go's
  notifications engine owns person-level delivery (§6.4).
  """

  require Logger

  alias AtlasOrchestrator.Dashboard.Signals, as: DashboardSignals
  alias AtlasOrchestrator.Finance.Signals
  alias AtlasOrchestrator.Realtime

  @board_topic "finance:board"
  @dashboard_topic "exec:dashboard"

  # Event type => {entity, id key}. The keys mirror the Go payloads in
  # internal/outbox/finance.go; the entity names match Go's aggregate constants
  # (expense, invoice, payment, budget). The optional keys those payloads carry
  # (`amount_minor`, `category_key`, `due_date`, `actual_minor`, …) are
  # deliberately not read: a signal is an id and a change type, never data.
  @events %{
    "finance.expense.submitted.v1" => {"expense", "expense_id"},
    "finance.expense.approved.v1" => {"expense", "expense_id"},
    "finance.expense.rejected.v1" => {"expense", "expense_id"},
    "finance.invoice.issued.v1" => {"invoice", "invoice_id"},
    "finance.invoice.overdue.v1" => {"invoice", "invoice_id"},
    "finance.payment.received.v1" => {"payment", "payment_id"},
    "finance.budget.allocated.v1" => {"budget", "budget_id"},
    "finance.budget.threshold_crossed.v1" => {"budget", "budget_id"}
  }

  @doc "Handles one event. Types it does not know are ignored, not failed."
  @spec handle_event(map()) :: :ok
  def handle_event(%{type: type, payload: payload} = event) do
    case Map.fetch(@events, type) do
      {:ok, {entity, id_key}} ->
        occurred_at = Map.get(event, :occurred_at)
        signal = Signals.entity_changed(entity, payload_value(payload, id_key), occurred_at)
        push(@board_topic, signal["type"], signal, type)

        dashboard = DashboardSignals.dashboard_changed(type, occurred_at)
        push(@dashboard_topic, dashboard["type"], dashboard, type)

      :error ->
        Logger.debug("finance event ignored", type: type)
    end

    :ok
  end

  def handle_event(_event), do: :ok

  defp push(topic, event, payload, type) do
    case Realtime.broadcast(topic, event, payload) do
      :ok ->
        :ok

      other ->
        Logger.warning("finance signal not delivered",
          topic: topic,
          type: type,
          result: inspect(other)
        )

        :ok
    end
  end

  defp payload_value(payload, key) when is_map(payload) do
    Map.get(payload, key) || Map.get(payload, String.to_atom(key))
  end

  defp payload_value(_payload, _key), do: nil
end
