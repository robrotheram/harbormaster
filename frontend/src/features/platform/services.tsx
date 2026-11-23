import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Server } from "lucide-react";
import { snapshotOptions } from "./api";
import { Button } from "@/components/ui/button";
import { Empty, ErrorNotice, Observation, State } from "@/shared/components";
import { date } from "@/shared/format";

function diagnostic(status: string, code?: string) {
  if (code === "HEALTH_CHECK_FAILED")
    return "This service did not pass its health check. Refresh to check again. If it remains unavailable, ask your administrator to check the service and its network connection.";
  if (code === "NOT_PROBED")
    return "No health check has been reported yet. Refresh shortly; if this continues, ask your administrator to check the health-check configuration.";
  if (code === "PROCESS_READY") return "Responding to health checks.";
  if (status === "healthy") return "Reported healthy by the platform.";
  return "No explanation was reported. Refresh to check again and share the support details with your administrator if the status persists.";
}

export function PlatformServices() {
  const query = useQuery(snapshotOptions());
  const services = query.data?.services?.items;
  return (
    <>
      <div className="page-title">
        <div>
          <h1>Platform services</h1>
          {query.isSuccess && (
            <p>Harbormaster {query.data.version || "version unavailable"}</p>
          )}
        </div>
        <Button
          variant="outline"
          size="sm"
          className="h-8"
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw className={query.isFetching ? "loading-refresh" : ""} />
          Refresh
        </Button>
      </div>
      <div className="list-toolbar">
        <span>
          {query.isError || query.isPending ? "—" : (services?.length ?? 0)}{" "}
          {services?.length === 1 && !query.isError ? "service" : "services"}
        </span>
        <Observation
          at={query.dataUpdatedAt}
          fetching={query.isFetching}
          error={query.isError}
        />
      </div>
      {query.isError ? (
        <ErrorNotice error={query.error} />
      ) : query.isPending ? (
        <p className="loading" role="status">
          Loading service observations…
        </p>
      ) : services?.length ? (
        <div className="service-inventory">
          <div className="service-inventory-heading">
            <span>Service</span>
            <span>Status</span>
            <span>Diagnostic</span>
          </div>
          {services.map((service) => (
            <div className="service-inventory-row" key={service.name}>
              <h2>
                <Server size={14} />
                {service.name}
              </h2>
              <State value={service.status} />
              <div className="service-diagnostic min-w-0 space-y-2">
                <p>{diagnostic(service.status, service.reason_code)}</p>
                {service.observed_at && (
                  <p>
                    <time dateTime={service.observed_at}>
                      Checked {date(service.observed_at)}
                    </time>
                  </p>
                )}
                {service.reason_code &&
                  service.reason_code !== "PROCESS_READY" && (
                    <details className="text-xs text-muted-foreground">
                      <summary className="cursor-pointer">
                        Support details
                      </summary>
                      <p className="mt-2 font-mono">
                        Code: {service.reason_code}
                      </p>
                    </details>
                  )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <Empty title="No service observations">
          Refresh to check whether the platform has reported service health.
        </Empty>
      )}
    </>
  );
}
