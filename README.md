# VIV-Backend
In order to run VIV backend, please follow this instructions:
1. Pull main branch into local machine
2. Modify file scripts/dev_env.sh:
   Change value of GOOGLE_APPLICATION_CREDENTIALS to wherever the credentials file is located within your local storage.
4. run scripts/dev_env.sh
5. execute go mod tidy
6. execute go run main.go

Then you will have VIV running in your local.
You can use then the postman collection tu test any endpoint.
