# Despliegue con contenedores — go-payment-methods-hub

## 1. Ejecución local

```bash
docker compose up -d --build      # API + Postgres
curl localhost:8081/healthz
open http://localhost:8081/docs   # Swagger UI
```

Sin contenedores:

```bash
go test ./... -race -count=1
go run ./cmd/api
```

## 2. La imagen

El `Dockerfile` usa **multi-stage build**:

1. **Etapa de compilación** (`golang:1.22-alpine`): descarga dependencias y compila con `CGO_ENABLED=0`, produciendo un binario estático.
2. **Etapa final** (`distroless/static-debian12:nonroot`): solo el binario.

Esto da tres propiedades que importan en un entorno de pagos:

- **Superficie de ataque mínima**: la imagen final no tiene shell, gestor de paquetes ni utilidades. Si alguien logra ejecución remota, no encuentra herramientas.
- **Sin root**: corre como `nonroot` (uid 65532).
- **Arranque rápido y peso bajo**: ~10-15 MB, lo que acelera el escalado horizontal y los despliegues.

```bash
docker build -t go-payment-methods-hub:1.0.0 .
docker run --rm -p 8081:8081 --env-file .env go-payment-methods-hub:1.0.0
```

## 3. Configuración

Toda la configuración entra por variables de entorno; **no hay secretos en la imagen ni en el repositorio**.

| Variable | Descripción | Por defecto |
|---|---|---|
| `APP_ENV` | Entorno lógico | `local` |
| `HTTP_PORT` | Puerto de escucha | `8081` |
| `DATABASE_URL` | Cadena de conexión a Postgres | — |
| `LOG_LEVEL` | `debug`/`info`/`warn`/`error` | `info` |
| `VAULT_ENCRYPTION_KEY` | Clave de cifrado del vault (mín. 32 bytes) | solo desarrollo |
| `PROVIDER_TIMEOUT_MS` | Timeout de llamadas a proveedores | `3000` |
| `CIRCUIT_BREAKER_THRESHOLD` | Fallos antes de abrir el circuito | `5` |
| `SHUTDOWN_TIMEOUT_SECONDS` | Margen de apagado ordenado | `15` |

En producción los secretos vienen de un gestor externo (AWS Secrets Manager, Vault, Kubernetes Secrets), inyectados en tiempo de ejecución.

## 4. Migraciones

Las migraciones son **versionadas** y se aplican como un paso previo y explícito al despliegue, nunca automáticamente al arrancar la aplicación:

```bash
psql "$DATABASE_URL" -f migrations/0001_init.sql
```

En Kubernetes, esto va como un `initContainer` o un `Job` que debe terminar antes de que el `Deployment` reciba tráfico.

## 5. Kubernetes

El servicio expone `/healthz` (liveness) y `/readyz` (readiness) por separado: el primero dice si el proceso está vivo, el segundo si puede recibir tráfico. Esta distinción evita que el orquestador reinicie un pod que solo está esperando a una dependencia.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payments-core
spec:
  replicas: 3
  selector:
    matchLabels: { app: payments-core }
  template:
    metadata:
      labels: { app: payments-core }
    spec:
      securityContext:
        runAsNonRoot: true
        seccompProfile: { type: RuntimeDefault }
      containers:
        - name: api
          image: ghcr.io/manuXD270516/go-payment-methods-hub:1.0.0
          ports: [{ containerPort: 8081 }]
          envFrom:
            - secretRef: { name: payments-core-secrets }
          livenessProbe:
            httpGet: { path: /healthz, port: 8081 }
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet: { path: /readyz, port: 8081 }
            initialDelaySeconds: 3
            periodSeconds: 5
          resources:
            requests: { cpu: 100m, memory: 64Mi }
            limits:   { cpu: 500m, memory: 256Mi }
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: { drop: ["ALL"] }
```

**Escalado horizontal:** el servicio no guarda estado en memoria que impida replicar; la serialización de operaciones concurrentes sobre un mismo pago se resuelve en la base de datos (versión optimista), no en el proceso. Por eso se puede escalar con un `HorizontalPodAutoscaler` sin riesgo de doble captura.

## 6. Apagado ordenado

Al recibir `SIGTERM`, el servicio deja de aceptar conexiones nuevas y espera a que terminen las peticiones en curso, hasta `SHUTDOWN_TIMEOUT_SECONDS`. Configurá `terminationGracePeriodSeconds` en Kubernetes por encima de ese valor para que el proceso alcance a cerrar limpio.

## 7. Checklist previo a producción

- [ ] Vault real (HSM o servicio certificado PCI) en lugar del adaptador AES local
- [ ] Adaptador Postgres en uso (no el de memoria) y migraciones aplicadas
- [ ] Secretos desde un gestor externo, nunca en la imagen
- [ ] TLS terminado en el balanceador o ingress
- [ ] Rate limiting en el borde
- [ ] Logs centralizados (el servicio ya emite JSON estructurado)
- [ ] Alertas sobre tasa de error y latencia
- [ ] Backups y prueba de restauración de la base
