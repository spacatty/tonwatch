# The Next.js dashboard, built as a standalone Node server.
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# Both are baked in at build time: PUBLIC_URL into the pages (Open Graph), FRAGWATCH_API into the
# /api rewrite. Changing either means rebuilding (docker compose up -d --build).
ARG PUBLIC_URL=
ARG FRAGWATCH_API=http://backend:8080
ENV PUBLIC_URL=$PUBLIC_URL FRAGWATCH_API=$FRAGWATCH_API NEXT_STANDALONE=1 NEXT_TELEMETRY_DISABLED=1
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production HOSTNAME=0.0.0.0 PORT=3000 NEXT_TELEMETRY_DISABLED=1
COPY --from=build --chown=node /app/.next/standalone ./
COPY --from=build --chown=node /app/.next/static ./.next/static
COPY --from=build --chown=node /app/public ./public
USER node
EXPOSE 3000
CMD ["node", "server.js"]
