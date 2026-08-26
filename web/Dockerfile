# UI server (static files via nginx, proxies /api/* to the API endpoint
# named by API_HOST/API_PORT — defaults match the "api" service in
# docker-compose.yml). nginx's docker-entrypoint envsubst's *.template
# files under /etc/nginx/templates into /etc/nginx/conf.d on container start.
FROM nginx:1.27-alpine
ENV API_HOST=api
ENV API_PORT=8080
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY . /usr/share/nginx/html
RUN rm -f /usr/share/nginx/html/Dockerfile /usr/share/nginx/html/nginx.conf.template

EXPOSE 80
