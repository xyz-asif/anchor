GoTodo API

Development
 - Hot reload (no restarts needed)
   - Install Air once: `go install github.com/air-verse/air@latest`
   - Start with live reload: `make air`
 - Swagger
   - Generate docs: `make swag`
   - Open UI: http://localhost:8080/swagger/index.html

docker run -d --name mongo -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=secret \
  mongo:6.0