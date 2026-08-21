// Package logging construye el logger estructurado (zap) del proyecto.
package logging

import "go.uber.org/zap"

// New devuelve un logger acorde al entorno: JSON en producción, legible en dev.
func New(env string) (*zap.Logger, error) {
	if env == "production" {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}
