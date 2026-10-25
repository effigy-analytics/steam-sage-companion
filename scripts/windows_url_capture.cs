// Synthetic per-user URL handler; never included in public artifacts.
using System;
using System.IO;
using System.Threading;
class Capture {
    [STAThread] static void Main(string[] args) {
        if (args.Length != 2) Environment.Exit(2);
        for (int attempt = 0; attempt < 100; attempt++) {
            try { File.AppendAllText(args[0], args[1] + "\n"); return; }
            catch (IOException) { Thread.Sleep(20); }
        }
        Environment.Exit(1);
    }
}
