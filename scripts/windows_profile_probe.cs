// Native profile diagnostics for disposable standard-user acceptance only.
using System;
using System.Runtime.InteropServices;
using System.Text;
namespace SageBoundary {
    public class FolderResult { public string HResult; public string Path; }
    public class ProfileResult { public int Error; public string Path; }
    public static class ProfileProbe {
        [DllImport("shell32.dll", ExactSpelling = true)]
        static extern int SHGetKnownFolderPath(ref Guid id, uint flags, IntPtr token, out IntPtr path);
        [DllImport("userenv.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        static extern bool GetUserProfileDirectoryW(IntPtr token, StringBuilder path, ref uint size);
        public static FolderResult Folder(string identifier, uint flags, IntPtr token) {
            Guid id = new Guid(identifier); IntPtr pointer;
            int result = SHGetKnownFolderPath(ref id, flags, token, out pointer);
            string path = pointer == IntPtr.Zero ? "" : Marshal.PtrToStringUni(pointer);
            if (pointer != IntPtr.Zero) Marshal.FreeCoTaskMem(pointer);
            return new FolderResult { HResult = unchecked((uint)result).ToString("X8"), Path = path };
        }
        public static ProfileResult Profile(IntPtr token) {
            uint size = 32768; StringBuilder path = new StringBuilder((int)size);
            bool success = GetUserProfileDirectoryW(token, path, ref size);
            return new ProfileResult { Error = success ? 0 : Marshal.GetLastWin32Error(), Path = success ? path.ToString() : "" };
        }
    }
}
