# GPVDP ERP — comandos de desarrollo local
.PHONY: help up down logs migrate seed test test-backend test-frontend e2e tidy fmt

help:            ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

up:              ## Levanta todo el stack (db, backend, frontend, adminer, mailhog). El backend migra y siembra solo.
	docker compose up --build

down:            ## Apaga el stack (conserva los datos)
	docker compose down

logs:            ## Sigue los logs del backend
	docker compose logs -f backend

migrate:         ## Aplica migraciones (el backend también lo hace al arrancar)
	docker compose run --rm backend -migrate-only

seed:            ## Siembra datos base (3 empresas + roles + admin)
	docker compose run --rm --entrypoint /app/seed backend

test: test-backend ## Corre las pruebas (backend; el frontend requiere Node)

test-backend:    ## Pruebas del backend Go
	cd backend && go test ./...

test-frontend:   ## Pruebas del frontend (requiere Node local o correr dentro de Docker)
	cd frontend && npm test

e2e:             ## Pruebas end-to-end (Playwright; requiere Node)
	cd frontend && npx playwright test

tidy:            ## go mod tidy del backend
	cd backend && go mod tidy

fmt:             ## Formatea el backend
	cd backend && gofmt -w .
