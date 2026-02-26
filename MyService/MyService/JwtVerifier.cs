using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace MyService;

/// <summary>
/// Verifies JWT tokens produced by MyAuth (format: base64(payload).base64(signature), HMAC-SHA256).
/// </summary>
public static class JwtVerifier
{
    /// <summary>Payload claims: login, role, session_id, expires_at (RFC3339), issued_at (RFC3339).</summary>
    public sealed class TokenClaims
    {
        [JsonPropertyName("login")]
        public string Login { get; set; } = "";
        [JsonPropertyName("role")]
        public string Role { get; set; } = "";
        [JsonPropertyName("session_id")]
        public string SessionId { get; set; } = "";
        [JsonPropertyName("expires_at")]
        public string ExpiresAt { get; set; } = "";
        [JsonPropertyName("issued_at")]
        public string IssuedAt { get; set; } = "";
    }

    /// <summary>
    /// Parses and verifies the token (signature + expiration). Returns claims or an error message.
    /// </summary>
    public static (TokenClaims? Claims, string? Error) ParseAndVerify(string token, byte[] secret)
    {
        if (string.IsNullOrWhiteSpace(token))
            return (null, "missing token");

        var parts = token.Split('.', 3);
        if (parts.Length != 2)
            return (null, "invalid token format");

        byte[] payloadBytes;
        try
        {
            payloadBytes = Convert.FromBase64String(parts[0]);
        }
        catch
        {
            return (null, "invalid payload encoding");
        }

        byte[] receivedSig;
        try
        {
            receivedSig = Convert.FromBase64String(parts[1]);
        }
        catch
        {
            return (null, "invalid signature encoding");
        }

        using var hmac = new HMACSHA256(secret);
        var expectedSig = hmac.ComputeHash(payloadBytes);
        if (expectedSig.Length != receivedSig.Length || !CryptographicOperations.FixedTimeEquals(expectedSig, receivedSig))
            return (null, "invalid token signature");

        TokenClaims? claims;
        try
        {
            var json = Encoding.UTF8.GetString(payloadBytes);
            claims = JsonSerializer.Deserialize<TokenClaims>(json);
        }
        catch
        {
            return (null, "invalid token payload");
        }

        if (claims == null)
            return (null, "invalid token payload");

        if (string.IsNullOrEmpty(claims.ExpiresAt))
            return (null, "missing expiration");

        if (!DateTime.TryParse(claims.ExpiresAt, null, System.Globalization.DateTimeStyles.RoundtripKind, out var expiresAt))
            return (null, "invalid expiration format");

        if (expiresAt.Kind == DateTimeKind.Unspecified)
            expiresAt = DateTime.SpecifyKind(expiresAt, DateTimeKind.Utc);

        if (expiresAt < DateTime.UtcNow)
            return (null, "token expired");

        return (claims, null);
    }
}
