// Native read-only fixture adapter. Never compiled into or shipped with Setup.
using System;
using System.Runtime.InteropServices;
using System.Runtime.InteropServices.ComTypes;
using System.Text;

namespace SagePackageFixtures {
    [ComImport, Guid("00021401-0000-0000-C000-000000000046")]
    internal class ShellLink { }

    [ComImport, Guid("000214F9-0000-0000-C000-000000000046"), InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
    internal interface IShellLinkW {
        [PreserveSig] int GetPath([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder path, int count, IntPtr findData, uint flags);
        [PreserveSig] int GetIDList(out IntPtr idList);
        [PreserveSig] int SetIDList(IntPtr idList);
        [PreserveSig] int GetDescription([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder text, int count);
        [PreserveSig] int SetDescription([MarshalAs(UnmanagedType.LPWStr)] string text);
        [PreserveSig] int GetWorkingDirectory([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder path, int count);
        [PreserveSig] int SetWorkingDirectory([MarshalAs(UnmanagedType.LPWStr)] string path);
        [PreserveSig] int GetArguments([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder arguments, int count);
        [PreserveSig] int SetArguments([MarshalAs(UnmanagedType.LPWStr)] string arguments);
        [PreserveSig] int GetHotkey(out short hotkey);
        [PreserveSig] int SetHotkey(short hotkey);
        [PreserveSig] int GetShowCmd(out int show);
        [PreserveSig] int SetShowCmd(int show);
        [PreserveSig] int GetIconLocation([Out, MarshalAs(UnmanagedType.LPWStr)] StringBuilder path, int count, out int index);
        [PreserveSig] int SetIconLocation([MarshalAs(UnmanagedType.LPWStr)] string path, int index);
        [PreserveSig] int SetRelativePath([MarshalAs(UnmanagedType.LPWStr)] string path, uint reserved);
        [PreserveSig] int Resolve(IntPtr window, uint flags);
        [PreserveSig] int SetPath([MarshalAs(UnmanagedType.LPWStr)] string path);
    }

    public sealed class ShortcutObservation {
        public int LoadHResult;
        public int ResolveHResult;
        public int PathHResult;
        public int ArgumentsHResult;
        public int WorkingDirectoryHResult;
        public string TargetPath;
        public string Arguments;
        public string WorkingDirectory;
    }

    public static class ShortcutReader {
        public static ShortcutObservation Read(string path) {
            object instance = new ShellLink();
            var result = new ShortcutObservation();
            try {
                try { ((IPersistFile)instance).Load(path, 0); }
                catch (COMException error) { result.LoadHResult = error.ErrorCode; return result; }
                var link = (IShellLinkW)instance;
                // SLR_NO_UI | NOUPDATE | NOSEARCH | NOTRACK, 1000 ms maximum.
                // This cannot launch, prompt, save the link or search other files.
                result.ResolveHResult = link.Resolve(IntPtr.Zero, (1000u << 16) | 0x39u);
                var target = new StringBuilder(1024);
                var arguments = new StringBuilder(4096);
                var working = new StringBuilder(1024);
                result.PathHResult = link.GetPath(target, target.Capacity, IntPtr.Zero, 0);
                result.ArgumentsHResult = link.GetArguments(arguments, arguments.Capacity);
                result.WorkingDirectoryHResult = link.GetWorkingDirectory(working, working.Capacity);
                result.TargetPath = target.ToString();
                result.Arguments = arguments.ToString();
                result.WorkingDirectory = working.ToString();
                return result;
            } finally { Marshal.FinalReleaseComObject(instance); }
        }
    }
}
