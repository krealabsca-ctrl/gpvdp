@echo off
setlocal
chcp 65001 >nul
title GPVDP - Respaldo de la base de datos

REM Respaldo de la base completa. Se puede correr con gente trabajando adentro:
REM pg_dump toma una foto consistente y no bloquea a nadie.
REM
REM Durante la prueba con los auxiliares, correrlo AL FINAL DE CADA DIA.
REM Es lo unico que permite volver atras si alguien clasifica o revisa mal en masa.

cd /d "%~dp0"

echo.
echo ============================================
echo   GPVDP - Respaldo de la base de datos
echo ============================================
echo.

docker compose ps db 2>nul | find "gpvdp_entrega-db-1" >nul
if errorlevel 1 (
  echo   El sistema no esta levantado.
  echo   Abri primero Levantar-GPVDP.bat y volve a intentar.
  echo.
  pause
  exit /b 1
)

if not exist "respaldos" mkdir "respaldos"

REM Fecha y hora en formato ordenable (independiente del formato regional de Windows).
for /f %%i in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd_HHmm"') do set STAMP=%%i
set ARCHIVO=respaldos\gpvdp_%STAMP%.dump

echo   Sacando el respaldo...
docker compose exec -T db pg_dump -U gpvdp -d gpvdp -Fc -f /tmp/respaldo.dump
if errorlevel 1 goto :error

docker cp gpvdp_entrega-db-1:/tmp/respaldo.dump "%ARCHIVO%"
if errorlevel 1 goto :error

docker compose exec -T db rm -f /tmp/respaldo.dump

echo.
echo   LISTO. Respaldo guardado en:
echo   %CD%\%ARCHIVO%
echo.
for %%A in ("%ARCHIVO%") do echo   Tamano: %%~zA bytes
echo.
echo   Consejo: copia este archivo a un disco externo o a otra carpeta.
echo   Si se dana la computadora, un respaldo que vive solo en ella no sirve.
echo ============================================
echo.
pause
exit /b 0

:error
echo.
echo   FALLO EL RESPALDO. No cierres esta ventana: mostrale el mensaje de arriba
echo   a quien te da soporte. NO abras el sistema a los auxiliares sin respaldo.
echo.
pause
exit /b 1
