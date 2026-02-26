using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace MyService;

/// <summary>
/// Service for registering MyService instance in MyDiscoverer with session assignment.
/// Uses MSShared.Config for configuration and MSShared.GetAssignedSession() for current session.
/// </summary>
public class DiscovererRegistrationService
{
    private readonly HttpClient _httpClient;

    public DiscovererRegistrationService(HttpClient httpClient)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
    }

    /// <summary>
    /// Registers or updates instance in MyDiscoverer with optional session assignment.
    /// </summary>
    private async Task<bool> RegisterAsync(string? assignedClientSessionId = null, CancellationToken cancellationToken = default)
    {
        var config = MSShared.Config;
        if (string.IsNullOrEmpty(config.DiscovererUrl))
        {
            return false;
        }

        try
        {
            var registerUrl = $"{config.DiscovererUrl.TrimEnd('/')}/v1/register";
            var payload = new RegisterRequestPayload(
                InstanceId: config.InstanceId,
                ServiceType: "grpcserver",
                Ipv4: config.DiscoverableHost,
                Port: config.Port,
                Timestamp: DateTime.UtcNow.ToString("o"),
                TtlMs: config.RegistrationTtlMs
            );

            var json = JsonSerializer.Serialize(payload);
            var content = new StringContent(json, Encoding.UTF8, "application/json");
            var response = await _httpClient.PostAsync(registerUrl, content, cancellationToken);

            if (response.IsSuccessStatusCode)
            {
                Console.WriteLine($"Register success: instance_id={config.InstanceId}, session_id={assignedClientSessionId ?? "none"}");
                return true;
            }
            else
            {
                var body = await response.Content.ReadAsStringAsync(cancellationToken);
                Console.WriteLine($"Register failed: {(int)response.StatusCode} {response.StatusCode}; body={body}");
                return false;
            }
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Exception registering: {ex.Message}");
            return false;
        }
    }

    /// <summary>
    /// Starts registration retry loop and heartbeat loop.
    /// Retries registration up to 100 times with 2 second intervals.
    /// After successful registration, starts infinite heartbeat loop that includes current assigned session.
    /// </summary>
    public void StartRegistrationAndHeartbeat()
    {
        _ = Task.Run(async () =>
        {
            var config = MSShared.Config;
            Console.WriteLine($"DiscovererURL={config.DiscovererUrl}");

            for (var i = 0; i < 100; i++)
            {
                try
                {
                    var success = await RegisterAsync(null);
                    if (success)
                    {
                        Console.WriteLine($"Register success: instance_id={config.InstanceId}, host={config.DiscoverableHost}, port={config.Port}");
                        // Heartbeat loop: re-register periodically so instance stays in EDS
                        // Include current session_id if one is assigned
                        while (true)
                        {
                            await Task.Delay(config.HeartbeatIntervalMs);
                            try
                            {
                                var currentSessionId = MSShared.GetAssignedSession();
                                await RegisterAsync(
                                    string.IsNullOrWhiteSpace(currentSessionId) ? null : currentSessionId
                                );
                            }
                            catch (Exception ex)
                            {
                                Console.WriteLine($"Heartbeat exception: {ex.Message}");
                            }
                        }
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Exception registering: {ex.Message}");
                }
                if (i < 99)
                {
                    Console.WriteLine("Retry in 2s");
                    await Task.Delay(2000);
                }
            }
        });
    }
}

/// <summary>Request body for MyDiscoverer POST /v1/register (OpenAPI: instance_id, service_type, ipv4, port, timestamp, ttl_ms).</summary>
internal record RegisterRequestPayload(
    [property: JsonPropertyName("instance_id")] string InstanceId,
    [property: JsonPropertyName("service_type")] string ServiceType,
    [property: JsonPropertyName("ipv4")] string Ipv4,
    [property: JsonPropertyName("port")] int Port,
    [property: JsonPropertyName("timestamp")] string Timestamp,
    [property: JsonPropertyName("ttl_ms")] int TtlMs
);
