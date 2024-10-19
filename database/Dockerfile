FROM mongo:latest

COPY init-mongo.js /docker-entrypoint-initdb.d/

ENV MONGO_INITDB_ROOT_USERNAME=admin
ENV MONGO_INITDB_ROOT_PASSWORD=adminpassword
ENV MONGO_INITDB_DATABASE=videoCallDB
