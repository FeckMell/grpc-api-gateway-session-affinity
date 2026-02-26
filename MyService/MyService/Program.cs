using MyService;
using Microsoft.AspNetCore.Server.Kestrel.Core;

var config = MSSConfig.Parse();
MSShared.Initialize(config);

var builder = WebApplication.CreateBuilder(args);
builder.WebHost.ConfigureKestrel(serverOptions =>
{
    serverOptions.ListenAnyIP(config.Port, listenOptions =>
    {
        listenOptions.Protocols = HttpProtocols.Http2;
    });
});
builder.Services.AddSingleton(config.JwtSecretBytes);
builder.Services.AddSingleton<AuthInterceptor>();
builder.Services.AddGrpc(o => o.Interceptors.Add<AuthInterceptor>());
builder.Services.AddGrpcHealthChecks();

// Register ApiEchoService
builder.Services.AddSingleton<ApiEchoService>();

var app = builder.Build();
app.Use(async (context, next) =>
{
    await next();
    Console.WriteLine($"HTTP {context.Request.Method} {context.Request.Path} -> {context.Response.StatusCode}");
});
app.MapGrpcService<ApiEchoService>();
app.MapGrpcHealthChecksService();
app.Lifetime.ApplicationStarted.Register(() =>
{
    Console.WriteLine("DEBUG: App started");
    var httpClient = new HttpClient();
    httpClient.Timeout = TimeSpan.FromSeconds(10);
    var registrationService = new DiscovererRegistrationService(httpClient);
    registrationService.StartRegistrationAndHeartbeat();
    // Note: HttpClient is not disposed here because StartRegistrationAndHeartbeat runs in background
    // and will use it indefinitely. This is acceptable for long-running background tasks.
});

await app.RunAsync();
