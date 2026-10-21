# Arquitectura de RelayDTL

El relayer mantiene seis dominios: cuentas y saldos, mensajes, rutas, recibos, confirmaciones y efectos de liquidación. `Relayer` orquesta las llamadas; `Ledger` es la fuente de verdad de disponible y retenido por activo. Los snapshots y eventos hacen observable el recorrido sin transporte de red.

```mermaid
flowchart LR
    M[MessageBook] --> R[RouteBook]
    R --> T[ReceiptBook]
    N[NodeRegistry] --> C[ConfirmationBook]
    T --> C
    C --> S[SettlementEngine]
    S --> E[ReceiptEffect]
    E --> L[Ledger]
    L --> A[AccountSnapshot]
```

## Secuencia de entrega

```mermaid
sequenceDiagram
    participant P as Pagador
    participant R as Relayer
    participant N as Nodos
    participant L as Ledger
    participant B as Beneficiario
    P->>R: SubmitMessage
    R->>L: hold(principal + fee_limit)
    R->>R: seleccionar ruta y aceptar recibo
    N->>R: confirmar entrega con peso
    R->>R: evaluar quórum y ventana
    R->>L: cargar principal + fee
    L->>B: acreditar principal
    R->>L: acreditar operador y devolver fee restante
```

Los IDs de mensaje, recibo, ruta y contexto identifican objetos distintos. El recibo captura un digest de la ruta; cada confirmación añade nodo, estado, peso y época. El `SettlementEngine` mantiene el estado del efecto económico de cada recibo, mientras el diario registra débitos, créditos, retenciones y liberaciones.

## Estados

`Message` pasa por abierto, encaminado, entregado y liquidado, o termina por expiración/cancelación. Un recibo aceptado puede liquidarse o quedar fuera de ventana. Las confirmaciones de nodos retirados o pausados no cuentan. La política fija quórum, peso de rechazo, TTL y tolerancia temporal.
