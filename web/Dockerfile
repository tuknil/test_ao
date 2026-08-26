# UI server (static files via nginx, proxies /api/* to the API endpoint
# named by API_ENDPOINT — defaults to the "api" service in
# docker-compose.yml, but can point at a hosted target such as an Azure
# Container Apps FQDN (e.g. https://ingestion-server.<env>.azurecontainerapps.io).
# nginx's docker-entrypoint envsubst's *.template files under
# /etc/nginx/templates into /etc/nginx/conf.d on container start.
FROM artifact.it.att.com/docker-hub/nginx:alpine
ENV GOPROXY=https://nm2553@att.com:***REMOVED-CREDENTIAL***@artifact.it.att.com/artifactory/api/go/att-go-public-group
ENV GONOSUMDB=off
ENV GOPRIVATE=it.att.com
#ENV API_ENDPOINT=http://api:8080
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY . /usr/share/nginx/html
RUN rm -f /usr/share/nginx/html/Dockerfile /usr/share/nginx/html/nginx.conf.template

EXPOSE 80
