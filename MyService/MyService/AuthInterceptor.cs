using Grpc.Core;
using Grpc.Core.Interceptors;

namespace MyService;

/// <summary>
/// gRPC server interceptor that validates JWT and session-id for MyServiceEcho, MyServiceSubscribe and MyServiceShutdown.
/// </summary>
public class AuthInterceptor : Interceptor
{
    private readonly byte[] _jwtSecret;
    private const string MyServiceEchoMethod = "/MyServiceEcho";
    private const string MyServiceSubscribeMethod = "/MyServiceSubscribe";
    private const string MyServiceShutdownMethod = "/MyServiceShutdown";
    private const string AuthorizationKey = "authorization";
    private const string SessionIdKey = "session-id";

    public AuthInterceptor(byte[] jwtSecret)
    {
        _jwtSecret = jwtSecret ?? Array.Empty<byte>();
    }

    private static bool RequiresAuth(string method)
    {
        return method.EndsWith(MyServiceEchoMethod, StringComparison.Ordinal)
            || method.EndsWith(MyServiceSubscribeMethod, StringComparison.Ordinal)
            || method.EndsWith(MyServiceShutdownMethod, StringComparison.Ordinal)
            ;
    }

    private static string? GetMetadataValue(Metadata headers, string key)
    {
        foreach (var e in headers)
        {
            if (string.Equals(e.Key, key, StringComparison.OrdinalIgnoreCase))
                return e.Value;
        }
        return null;
    }

    private void ValidateAuth(ServerCallContext context)
    {
        if (_jwtSecret.Length == 0)
        {
            throw new RpcException(new Status(StatusCode.Unauthenticated, "JWT_SECRET not configured"));
        }

        var token = GetMetadataValue(context.RequestHeaders, AuthorizationKey);
        if (string.IsNullOrWhiteSpace(token))
        {
            throw new RpcException(new Status(StatusCode.Unauthenticated, "missing or invalid authorization"));
        }

        token = token.Trim();
        var (claims, error) = JwtVerifier.ParseAndVerify(token, _jwtSecret);
        if (error != null)
        {
            throw new RpcException(new Status(StatusCode.Unauthenticated, error));
        }

        var sessionIdHeader = GetMetadataValue(context.RequestHeaders, SessionIdKey);
        if (claims!.SessionId != sessionIdHeader)
        {
            throw new RpcException(new Status(StatusCode.Unauthenticated, "session_id mismatch"));
        }
    }

    public override Task<TResponse> UnaryServerHandler<TRequest, TResponse>(
        TRequest request,
        ServerCallContext context,
        UnaryServerMethod<TRequest, TResponse> continuation)
    {
        if (RequiresAuth(context.Method))
            ValidateAuth(context);
        return continuation(request, context);
    }

    public override Task ServerStreamingServerHandler<TRequest, TResponse>(
        TRequest request,
        IServerStreamWriter<TResponse> responseStream,
        ServerCallContext context,
        ServerStreamingServerMethod<TRequest, TResponse> continuation)
    {
        if (RequiresAuth(context.Method))
            ValidateAuth(context);
        return continuation(request, responseStream, context);
    }
}
