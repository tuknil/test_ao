# UI server (static files via nginx, proxies /api/* to the "api" service)
FROM artifact.it.att.com/docker-hub/nginx:alpine
ENV GOPROXY=https://nm2553@att.com:***REMOVED-CREDENTIAL***@artifact.it.att.com/artifactory/api/go/att-go-public-group
ENV GONOSUMDB=off
ENV GOPRIVATE=it.att.com
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY . /usr/share/nginx/html
RUN rm -f /usr/share/nginx/html/Dockerfile /usr/share/nginx/html/nginx.conf

EXPOSE 80
