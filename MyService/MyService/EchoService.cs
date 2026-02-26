using Grpc.Core;
using MyProto;

namespace MyService;

public class ApiEchoService : MyServiceAPI.MyServiceAPIBase
{
    private const string ServerValueConstant = "my_service";
    private readonly string _podName = MSShared.Config.InstanceId;

    public override async Task<EchoResponse> MyServiceEcho(EchoRequest request, ServerCallContext context)
    {
        Console.WriteLine($"Received MyServiceEcho {request.Value}");
        var clientSessionId = ValidateAndSetClientSession(context);
        var serverSessionId = MSShared.GetAssignedSession();
        return BuildEchoResponse(request.Value, serverSessionId ?? "", clientSessionId, 0, "MyServiceEcho");
    }

    public override async Task MyServiceSubscribe(EchoRequest request, IServerStreamWriter<EchoResponse> responseStream, ServerCallContext context)
    {
        Console.WriteLine($"Received MyServiceSubscribe {request.Value}");

        var clientSessionId = ValidateAndSetClientSession(context);

        try
        {
            var index = 0;
            while (!context.CancellationToken.IsCancellationRequested)
            {
                var serverSessionId = MSShared.GetAssignedSession();
                var response = BuildEchoResponse(request.Value, serverSessionId ?? "", clientSessionId, index, "MyServiceSubscribe");
                await responseStream.WriteAsync(response, context.CancellationToken);
                index++;
                await Task.Delay(TimeSpan.FromSeconds(5), context.CancellationToken);
            }
        }
        catch (OperationCanceledException)
        {
            // Client disconnected or cancelled
        }
    }

    public override Task<ShutdownResponse> MyServiceShutdown(ShutdownRequest request, ServerCallContext context)
    {
        // Check if server session is empty
        var serverSessionId = MSShared.GetAssignedSession();
        if (string.IsNullOrWhiteSpace(serverSessionId))
        {
            throw new RpcException(new Status(StatusCode.PermissionDenied, "Server session is not set"));
        }

        // Check if client session matches server session
        var clientSessionId = GetClientSessionIdFromContext(context);
        if (clientSessionId != serverSessionId)
        {
            throw new RpcException(new Status(StatusCode.PermissionDenied, $"Client session mismatch: client={clientSessionId ?? "none"}, server={serverSessionId}"));
        }

        var response = new ShutdownResponse
        {
            PodName = _podName,
            ServerSessionId = serverSessionId ?? "",
            ClientSessionId = clientSessionId ?? "",
            Index = 0,
            ServerMethod = "MyServiceShutdown"
        };
        // Exit after response is sent; short delay so gRPC can flush the response
        _ = Task.Run(() =>
        {
            Thread.Sleep(200);
            Environment.Exit(0);
        });
        return Task.FromResult(response);
    }

    private static string? GetClientSessionIdFromContext(ServerCallContext context)
    {
        foreach (var e in context.RequestHeaders)
        {
            if (string.Equals(e.Key, "session-id", StringComparison.OrdinalIgnoreCase))
                return e.Value;
        }
        return null;
    }

    private EchoResponse BuildEchoResponse(string clientValue, string serverSessionId, string? clientSessionId, int index, string serverMethod)
    {
        return new EchoResponse
        {
            ClientValue = clientValue,
            ServerValue = ServerValueConstant,
            PodName = _podName,
            ServerSessionId = serverSessionId ?? "",
            ClientSessionId = clientSessionId ?? "",
            Index = index,
            ServerMethod = serverMethod
        };
    }

    private string ValidateAndSetClientSession(ServerCallContext context)
    {
        var clientSessionId = GetClientSessionIdFromContext(context);
        if (string.IsNullOrWhiteSpace(clientSessionId))
        {
            throw new RpcException(new Status(StatusCode.InvalidArgument, "session-id header is required"));
        }
    
        string? currentSessionId = MSShared.GetAssignedSession();
        bool isNewSession = string.IsNullOrWhiteSpace(currentSessionId);
        if (!isNewSession && currentSessionId != clientSessionId)
        {
            throw new RpcException(new Status(StatusCode.Internal, $"Session conflict: was={currentSessionId}, now={clientSessionId}"));
        }

        MSShared.SetAssignedSession(clientSessionId);
        return clientSessionId;
    }
}
