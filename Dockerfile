FROM golang:1.19 AS build
RUN mkdir /app
ADD . /app
WORKDIR /app
RUN apt update && apt install -y make
RUN make
CMD ["/app/main"]