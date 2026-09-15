@echo off
title GPVDP ERP - Apagar
cd /d "%~dp0"
echo Apagando GPVDP ERP (los datos se conservan)...
docker compose down
echo.
echo Listo. Para volver a levantarlo: doble clic en "Levantar-GPVDP.bat".
pause
