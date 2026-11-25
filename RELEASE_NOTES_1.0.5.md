# Lair v1.0.5

Parche sobre v1.0.4.

## Corregido

- **Panic al instalar Node con nvm**: `Manager.Run` inicia el trabajo en una
  goroutine antes de devolver el `*Job`, y el callback `OnSuccess` capturaba la
  variable del llamador, que podía seguir siendo `nil`. El callback recibe
  ahora el job explícitamente (`OnSuccess(job *Job)`). Afectaba a la instalación
  de Node por nvm y hacía fallar la suite `internal/engine` en CI.

## Verificación

- `go test ./...` y `go test -count=10 ./internal/engine ./internal/installer`
  sin errores.
- `npm test`, `typecheck`, `i18n:check`, `actions:check` y `build` sin errores.
