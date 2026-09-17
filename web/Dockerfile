# UI server (static files via nginx, proxies /api/* to the "api" service)
FROM artifact.it.att.com/docker-hub/nginx:alpine
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY . /usr/share/nginx/html
RUN rm -f /usr/share/nginx/html/Dockerfile /usr/share/nginx/html/nginx.conf.template

EXPOSE 80
