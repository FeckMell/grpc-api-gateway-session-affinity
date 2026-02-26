using System.Net;
using System.Net.NetworkInformation;
using System.Net.Sockets;

namespace MyService;

/// <summary>
/// Helper class for network operations, particularly for auto-detecting container's IPv4 address.
/// </summary>
public static class NetworkHelper
{
    /// <summary>
    /// Auto-detects the primary non-loopback IPv4 address of the current machine/container.
    /// Returns the first suitable IPv4 address found on an active network interface.
    /// </summary>
    /// <returns>IPv4 address as string, or null if no suitable address found</returns>
    public static string? GetPrimaryIPv4Address()
    {
        try
        {
            var interfaces = NetworkInterface.GetAllNetworkInterfaces();
            
            foreach (var networkInterface in interfaces)
            {
                // Skip loopback and tunnel interfaces
                if (networkInterface.NetworkInterfaceType == NetworkInterfaceType.Loopback ||
                    networkInterface.NetworkInterfaceType == NetworkInterfaceType.Tunnel)
                {
                    continue;
                }

                // Skip interfaces that are not up
                if (networkInterface.OperationalStatus != OperationalStatus.Up)
                {
                    continue;
                }

                var properties = networkInterface.GetIPProperties();
                foreach (var address in properties.UnicastAddresses)
                {
                    // Only IPv4 addresses
                    if (address.Address.AddressFamily != AddressFamily.InterNetwork)
                    {
                        continue;
                    }

                    // Skip loopback addresses
                    if (IPAddress.IsLoopback(address.Address))
                    {
                        continue;
                    }

                    // Found a suitable IPv4 address
                    return address.Address.ToString();
                }
            }

            // No suitable address found
            return null;
        }
        catch (Exception ex)
        {
            Console.WriteLine($"Error auto-detecting IPv4 address: {ex.Message}");
            return null;
        }
    }
}
