dapr stop -f . &&
lsof -i:8080 | grep main | awk '{print $2}' | xargs kill &&
lsof -i:8081 | grep model | awk '{print $2}' | xargs kill