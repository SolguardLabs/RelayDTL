# Seguridad

## Modelo

RelayDTL asume nodos registrados con pesos de confirmacion, rutas configuradas
por politica operativa y cuentas con saldo por asset. El motor debe conservar
los fondos reservados, rechazar receipts fuera de ventana y liquidar solo cuando
las confirmaciones observadas alcanzan quorum.

## Invariantes Esperadas

- El saldo total por asset se conserva entre cuentas, escrow y reservas.
- Un mensaje no puede liquidarse antes de cumplir quorum.
- Los receipts fuera de expiracion no son admitidos para settlement.
- Las rutas deben respetar capacidad, origen, destino, asset y ventanas.
- Las confirmaciones deben proceder de nodos registrados y activos.
- Los fees liquidados no pueden superar el limite economico del mensaje.

## Validacion

La suite local ejecuta tests Go y escenarios TypeScript sobre el binario. CI
aplica formato, tests, `go vet` y tests Node. Dependabot mantiene actualizadas
las dependencias de Go, npm y GitHub Actions.

## Alcance De Revision

La revision debe cubrir:

- `src/ledger*` para accounting y reservas.
- `src/settlement*` para finalizacion de receipts.
- `src/confirmation*` para quorum y estados.
- `src/route*` para seleccion y admision de rutas.
- `src/scenario*` para interpretacion de entradas JSON.

## Reporte Interno

Los reportes deben incluir resumen, severidad, precondiciones, impacto
economico, pasos conceptuales, mitigacion y tests recomendados.
