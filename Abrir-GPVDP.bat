@echo off
setlocal
chcp 65001 >nul
title GPVDP - Abrir el sistema

REM Abre el sistema en el navegador, SIEMPRE por 127.0.0.1 y no por "localhost".
REM
REM Por que importa: "localhost" y "127.0.0.1" son el mismo lugar, pero para el navegador son
REM dos SITIOS distintos, cada uno con su propia memoria. Cuando la configuracion cambia, una
REM ventana vieja de "localhost" puede seguir usando la version anterior guardada y fallar con
REM "No se pudo conectar con el servidor" aunque el sistema este perfecto.
REM
REM Este acceso directo evita ese problema de raiz y ademas fuerza una carga limpia.

cd /d "%~dp0"

docker compose ps frontend 2>nul | find "gpvdp_entrega-frontend-1" >nul
if errorlevel 1 (
  echo.
  echo   El sistema no esta levantado.
  echo   Abri primero Levantar-GPVDP.bat y volve a intentar.
  echo.
  pause
  exit /b 1
)

echo.
echo   Abriendo GPVDP en el navegador...
echo   http://127.0.0.1:5173
echo.

REM El parametro extra fuerza al navegador a no reutilizar una version guardada.
start "" "http://127.0.0.1:5173/login?nuevo=%RANDOM%"

echo   Si la pantalla dice "No se pudo conectar con el servidor":
echo     1. Presiona Ctrl+Shift+R en esa ventana.
echo     2. Si sigue igual, cerra TODAS las ventanas del navegador y volve a abrir este archivo.
echo.
timeout /t 6 >nul
exit /b 0
