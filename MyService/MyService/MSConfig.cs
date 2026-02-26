using System.Text;

namespace MyService;

/// <summary>
/// MyService configuration parsed from environment variables.
/// Use <see cref="Parse"/> to load and validate; on validation failure logs to console and exits.
/// </summary>
public class MSSConfig
{
    public int Port { get; }
    public string DiscovererUrl { get; }
    public string InstanceId { get; }
    public string DiscoverableHost { get; }
    public int RegistrationTtlMs { get; }
    public int HeartbeatIntervalMs { get; }
    public byte[] JwtSecretBytes { get; }

    private MSSConfig(
        int port,
        string discovererUrl,
        string instanceId,
        string discoverableHost,
        int registrationTtlMs,
        int heartbeatIntervalMs,
        byte[] jwtSecretBytes)
    {
        Port = port;
        DiscovererUrl = discovererUrl;
        InstanceId = instanceId;
        DiscoverableHost = discoverableHost;
        RegistrationTtlMs = registrationTtlMs;
        HeartbeatIntervalMs = heartbeatIntervalMs;
        JwtSecretBytes = jwtSecretBytes;
    }

    /// <summary>
    /// Reads and validates configuration from environment variables.
    /// On validation error: writes to Console.Error and calls Environment.Exit(exitCode).
    /// </summary>
    public static MSSConfig Parse()
    {
        static void ValidateAndExit(string message, int exitCode = 1)
        {
            Console.Error.WriteLine($"Configuration error: {message}");
            Environment.Exit(exitCode);
        }

        // SERVICE_PORT_GRPC
        var portStr = Environment.GetEnvironmentVariable("SERVICE_PORT_GRPC");
        int port;
        if (string.IsNullOrEmpty(portStr))
        {
            port = 5000;
        }
        else if (!int.TryParse(portStr, out port) || port <= 0 || port > 65535)
        {
            ValidateAndExit($"SERVICE_PORT_GRPC must be a valid port number (1-65535), got: {portStr}");
            return null!; // unreachable
        }

        // MY_DISCOVERER_URL
        var discovererUrl = Environment.GetEnvironmentVariable("MY_DISCOVERER_URL")?.TrimEnd('/');
        if (string.IsNullOrEmpty(discovererUrl))
        {
            ValidateAndExit("MY_DISCOVERER_URL is required but not set");
            return null!;
        }

        // HOSTNAME (instance id)
        var instanceId = Environment.GetEnvironmentVariable("HOSTNAME");
        if (string.IsNullOrWhiteSpace(instanceId))
        {
            ValidateAndExit("HOSTNAME is required but not set");
            return null!;
        }

        // Discoverable host: auto-detect IPv4
        var discoverableHost = NetworkHelper.GetPrimaryIPv4Address();
        if (string.IsNullOrWhiteSpace(discoverableHost))
        {
            ValidateAndExit("Could not auto-detect primary IPv4 address");
            return null!;
        }

        // REGISTRATION_TTL_MS
        var registrationTtlMs = 300000;
        var ttlStr = Environment.GetEnvironmentVariable("REGISTRATION_TTL_MS");
        if (!string.IsNullOrEmpty(ttlStr))
        {
            if (!int.TryParse(ttlStr, out var ttl) || ttl <= 0 || ttl > 600000)
            {
                ValidateAndExit($"REGISTRATION_TTL_MS must be between 1 and 600000, got: {ttlStr}");
                return null!;
            }
            registrationTtlMs = ttl;
        }

        // REGISTRATION_HEARTBEAT_INTERVAL_MS
        var heartbeatIntervalMs = 120000;
        var hbStr = Environment.GetEnvironmentVariable("REGISTRATION_HEARTBEAT_INTERVAL_MS");
        if (!string.IsNullOrEmpty(hbStr))
        {
            if (!int.TryParse(hbStr, out var hb) || hb <= 0)
            {
                ValidateAndExit($"REGISTRATION_HEARTBEAT_INTERVAL_MS must be greater than 0, got: {hbStr}");
                return null!;
            }
            heartbeatIntervalMs = hb;
        }

        // JWT_SECRET (required)
        var jwtSecret = Environment.GetEnvironmentVariable("JWT_SECRET");
        if (string.IsNullOrWhiteSpace(jwtSecret))
        {
            ValidateAndExit("JWT_SECRET is required but not set");
            return null!;
        }
        var jwtSecretBytes = Encoding.UTF8.GetBytes(jwtSecret);

        return new MSSConfig(
            port,
            discovererUrl,
            instanceId,
            discoverableHost,
            registrationTtlMs,
            heartbeatIntervalMs,
            jwtSecretBytes);
    }
}
