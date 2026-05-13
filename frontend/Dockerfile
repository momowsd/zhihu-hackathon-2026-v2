FROM docker.m.daocloud.io/library/node:24-alpine AS build

WORKDIR /src
RUN sed -i 's|dl-cdn.alpinelinux.org|mirrors.aliyun.com|g' /etc/apk/repositories
COPY package*.json ./
RUN npm config set registry https://registry.npmmirror.com && npm install
COPY . .
RUN npm run build

FROM docker.m.daocloud.io/library/nginx:1.27-alpine
RUN mkdir -p /etc/nginx/ssl-config
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY nginx-https.conf /etc/nginx/ssl-config/default-https.conf
COPY docker-entrypoint.d/99-enable-ssl.sh /docker-entrypoint.d/99-enable-ssl.sh
RUN chmod +x /docker-entrypoint.d/99-enable-ssl.sh
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 80 443
