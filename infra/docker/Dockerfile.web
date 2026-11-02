# syntax=docker/dockerfile:1

# ── Stage 1: build ─────────────────────────────────────────────
FROM node:24-alpine AS build

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# ── Stage 2: run ───────────────────────────────────────────────
FROM node:24-alpine

WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1

COPY package.json package-lock.json ./
RUN npm ci --omit=dev

COPY --from=build --chown=node:node /app/.next ./.next
COPY --from=build --chown=node:node /app/public ./public
COPY --from=build /app/next.config.ts ./next.config.ts

# `next start` binds to HOSTNAME when it is set. Inside a container the default
# is loopback, which is unreachable from the edge, so bind all interfaces.
# Railway injects PORT; 3000 is the local/CI default.
ENV HOSTNAME=0.0.0.0 \
    PORT=3000

EXPOSE 3000
USER node
CMD ["npm", "run", "start"]