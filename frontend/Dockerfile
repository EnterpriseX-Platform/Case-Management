FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
# cache downloads between builds; tolerate slow registries
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund --prefer-offline --fetch-retries=5 --fetch-timeout=600000
COPY . .
ARG API_URL=http://api:8080
ENV API_URL=$API_URL
RUN npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
EXPOSE 3000
CMD ["node", "server.js"]
