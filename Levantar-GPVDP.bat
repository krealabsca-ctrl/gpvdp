@echo off
title GPVDP ERP - Levantar
cd /d "%~dp0"

echo ============================================
echo   GPVDP ERP  -  Levantando el sistema
echo ============================================
echo.

rem Verifica que Docker este corriendo
docker info >nul 2>&1
if errorlevel 1 (
  echo [!] Docker no responde. Abri "Docker Desktop" y espera a que diga "Running".
  echo     Luego volve a ejecutar este archivo.
  echo.
  pause
  exit /b 1
)

echo Construyendo y levantando contenedores (puede tardar la primera vez)...
docker compose up -d --build
if errorlevel 1 (
  echo.
  echo [!] Hubo un error al levantar. Revisa el mensaje de arriba.
  pause
  exit /b 1
)

echo.
echo Esperando a que el sistema responda...
timeout /t 5 >nul

echo.
echo ============================================
echo   LISTO. Accesos:
echo ============================================
echo   Sistema (web):   http://localhost:5173
echo   API (backend):   http://localhost:8080/v1/healthz
echo   Base de datos:   http://localhost:8081   (Adminer)
echo   Correos (CxP):   http://localhost:8025   (MailHog)
echo.
echo.
echo   Uso LOCAL: solo esta computadora. Nadie mas en la red puede entrar.
echo ============================================
echo.

start http://localhost:5173
echo (Esta ventana se puede cerrar. El sistema queda corriendo.)
pause
