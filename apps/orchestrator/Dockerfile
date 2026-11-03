# syntax=docker/dockerfile:1

# ── Stage 1: build a release ───────────────────────────────────
FROM elixir:1.20-otp-29-alpine AS build

WORKDIR /app

ENV MIX_ENV=prod

RUN mix local.hex --force && mix local.rebar --force

# Cache the dependency graph before copying source.
COPY mix.exs mix.lock ./
COPY config config
RUN mix deps.get --only prod && mix deps.compile

COPY lib lib
COPY priv priv

RUN mix compile && mix release

# ── Stage 2: run ───────────────────────────────────────────────
FROM alpine:3.20

# A release built on Alpine needs the runtime libraries of the Erlang VM it
# bundles: ERTS links against libstdc++, ncurses and OpenSSL. Without these the
# release aborts at boot with a missing shared object.
RUN apk add --no-cache libstdc++ ncurses-libs openssl ca-certificates \
    && adduser -D -H -u 1000 appuser

COPY --from=build --chown=appuser /app/_build/prod/rel/atlas_orchestrator /app

# The orchestrator is a stateless websocket/logic service: it never needs an
# Erlang distribution or remote shell, and starting epmd in a container adds a
# failure mode for nothing. Operators use logs and health probes.
ENV RELEASE_DISTRIBUTION=none \
    MIX_ENV=prod

USER appuser
EXPOSE 4000
ENTRYPOINT ["/app/bin/atlas_orchestrator"]
CMD ["start"]