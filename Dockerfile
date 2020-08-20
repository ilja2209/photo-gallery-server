FROM ubuntu
WORKDIR /opt
RUN apt update -y && apt upgrade -y && apt install -y wget
RUN wget https://github.com/ilja2209/photo-gallery-server/releases/download/v1.0-alpha/server
RUN chmod 777 server
EXPOSE 5000
CMD ./server
