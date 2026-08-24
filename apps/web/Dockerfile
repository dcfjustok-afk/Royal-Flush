FROM node:22-alpine AS build

WORKDIR /workspace
COPY package.json package-lock.json tsconfig.base.json ./
COPY packages/contracts/package.json packages/contracts/tsconfig.json ./packages/contracts/
COPY packages/contracts/src ./packages/contracts/src
COPY apps/admin/package.json ./apps/admin/package.json
COPY apps/web/package.json apps/web/tsconfig.json apps/web/vite.config.ts apps/web/index.html ./apps/web/
COPY apps/web/src ./apps/web/src
RUN npm ci
ENV VITE_USE_API=true
RUN npm run build -w @royal-flush/web

FROM nginx:1.27-alpine
COPY deploy/nginx/player.conf /etc/nginx/conf.d/default.conf
COPY --from=build /workspace/apps/web/dist /usr/share/nginx/html
EXPOSE 80
