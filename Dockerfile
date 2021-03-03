FROM golang:1.14.3

WORKDIR /app

COPY . .

RUN go install 
RUN go build

# runs the main app
ENTRYPOINT /app/chat

EXPOSE 8000

