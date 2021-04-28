test_backend:
    #docker compose -f testing_env.yaml --env-file ./.env up -d --build
    docker-compose -f testing_env.yaml --env-file ./.env up -d --build
    go test ./app