# Frontend GPVDP — dev server dentro de Docker (Node NO está en la máquina host).
FROM node:24-alpine

WORKDIR /app

# 1) Dependencias primero (mejor caché de capas).
COPY package*.json ./
RUN npm install

# 2) Resto del código.
COPY . .

# 3) Generar el cliente tipado de Bancos desde el OpenAPI incluido en ./openapi.
#    (El contexto de build es solo ./frontend, por eso el YAML vive aquí.)
RUN npm run gen:api

EXPOSE 5173

# --host 0.0.0.0 para que sea accesible fuera del contenedor.
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0", "--port", "5173"]
