namespace MyService;

/// <summary>
/// Static class for shared MyService configuration and state.
/// Must be initialized before use via Initialize().
/// </summary>
public static class MSShared
{
    public static MSSConfig Config { get; private set; } = null!;
    private static string? _assignedSession;
    private static readonly object _sessionLock = new object();

    /// <summary>
    /// Initializes the shared configuration. Must be called before using MSShared.
    /// </summary>
    public static void Initialize(MSSConfig config)
    {
        Config = config ?? throw new ArgumentNullException(nameof(config));
    }

    /// <summary>
    /// Gets the current assigned session ID (thread-safe).
    /// </summary>
    public static string? GetAssignedSession()
    {
        lock (_sessionLock)
        {
            return _assignedSession;
        }
    }

    /// <summary>
    /// Sets the assigned session ID (thread-safe).
    /// </summary>
    public static void SetAssignedSession(string? sessionId)
    {
        lock (_sessionLock)
        {
            _assignedSession = sessionId;
        }
    }

    /// <summary>
    /// Checks if a session is currently assigned (thread-safe).
    /// </summary>
    public static bool HasAssignedSession()
    {
        lock (_sessionLock)
        {
            return !string.IsNullOrWhiteSpace(_assignedSession);
        }
    }
}
