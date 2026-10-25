; -*- coding: utf-8 -*-
Unicode true
RequestExecutionLevel user
SetCompressor zlib
SetOverwrite off
!include "LogicLib.nsh"
!include "x64.nsh"
!include "ownership.nsh"

Name "Steam Sage Companion"
OutFile "${OUTPUT}"
BrandingText "Steam Sage Companion preview"
ShowInstDetails show
ShowUninstDetails show
LicenseData "${REPO}\LICENSE"
Page license
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

!ifdef QA_ROOT
  !define UNINSTALL_KEY "Software\SteamSageCompanionQA\${QA_ID}\Uninstall"
  !define RUN_KEY "Software\SteamSageCompanionQA\${QA_ID}\Run"
!else
  !define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\SteamSageCompanion"
  !define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!endif

Var InstallDir
Var MenuDir
Var ConfigDir
Var LocalBase
Var Stage
Var OperationLock
Var OwnerSID
Var WasRunning
Var Leftovers
Var RollbackError
Var RegistryTouched
Var OldName
Var OldVersion
Var OldPublisher
Var OldUninstall
Var OldLocation
Var LedgerMoved
Var LedgerCommitted
!insertmacro DeclareFileState exe
!insertmacro DeclareFileState uninstaller
!insertmacro DeclareFileState license
!insertmacro DeclareFileState notices
!insertmacro DeclareFileState release
!insertmacro DeclareFileState launch
!insertmacro DeclareFileState settings
!ifdef QA_ROOT
Var QA_JournalWrites
Var QA_JournalLock
!endif

!macro Initialize PREFIX
Function ${PREFIX}.onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_OK|MB_ICONSTOP "This preview requires 64-bit Windows."
    SetErrorLevel 1
    Quit
  ${EndIf}
  SetShellVarContext current
  SetRegView 64
!ifdef QA_ROOT
  StrCpy $0 "${QA_ROOT}"
  System::Call 'kernel32::GetLongPathNameW(w r0, w .r1, i ${NSIS_MAX_STRLEN}) i.r2'
  StrCmp $2 0 initialization_failed
  StrCpy $LocalBase $1
  StrCpy $InstallDir "$LocalBase\Programs\Steam Sage Companion"
!ifdef QA_MENU_ROOT
  StrCpy $0 "${QA_MENU_ROOT}"
  System::Call 'kernel32::GetLongPathNameW(w r0, w .r1, i ${NSIS_MAX_STRLEN}) i.r2'
  StrCmp $2 0 initialization_failed
  StrCpy $MenuDir "$1\Steam Sage Companion"
!else
  StrCpy $MenuDir "$LocalBase\Start Menu\Steam Sage Companion"
!endif
  StrCpy $ConfigDir "$LocalBase\Config"
!else
  StrCpy $0 "$LOCALAPPDATA"
  System::Call 'kernel32::GetLongPathNameW(w r0, w .r1, i ${NSIS_MAX_STRLEN}) i.r2'
  StrCmp $2 0 initialization_failed
  StrCpy $LocalBase $1
  StrCpy $InstallDir "$LocalBase\Programs\Steam Sage Companion"
  StrCpy $0 "$SMPROGRAMS"
  System::Call 'kernel32::GetLongPathNameW(w r0, w .r1, i ${NSIS_MAX_STRLEN}) i.r2'
  StrCmp $2 0 initialization_failed
  StrCpy $MenuDir "$1\Steam Sage Companion"
  StrCpy $0 "$APPDATA"
  System::Call 'kernel32::GetLongPathNameW(w r0, w .r1, i ${NSIS_MAX_STRLEN}) i.r2'
  StrCmp $2 0 initialization_failed
  StrCpy $ConfigDir "$1\steam-sage-companion"
!endif
  StrCpy $INSTDIR $InstallDir
  ; Obtain the current token SID without usernames, environment substitutions,
  ; or calling an installed program. Object existence is the operation lock.
  System::Call 'advapi32::OpenProcessToken(p -1, i 8, *p.r0) i.r1'
  StrCmp $1 0 initialization_failed
  System::Alloc 1024
  Pop $2
  System::Call 'advapi32::GetTokenInformation(p r0, i 1, p r2, i 1024, *i.r3) i.r1'
  System::Call 'kernel32::CloseHandle(p r0)'
  ${If} $1 == 0
    System::Free $2
    Goto initialization_failed
  ${EndIf}
  System::Call '*$2(p.r3)'
  System::Call 'advapi32::ConvertSidToStringSidW(p r3, *p.r4) i.r1'
  System::Free $2
  StrCmp $1 0 initialization_failed
  System::Call '*$4(&w${NSIS_MAX_STRLEN} .r5)'
  StrCpy $OwnerSID $5
  System::Call 'kernel32::LocalFree(p r4)'
!ifdef QA_ROOT
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Global\SteamSageCompanion-Package-$OwnerSID-${QA_ID}") p.r0 ?e'
!else
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Global\SteamSageCompanion-Package-$OwnerSID") p.r0 ?e'
!endif
  Pop $1
  StrCpy $OperationLock $0
  StrCmp $0 0 initialization_failed
  ${If} $1 == 183
    DetailPrint "Another companion package operation is in progress."
    SetErrorLevel 1618
    Quit
  ${EndIf}
!ifdef QA_ROOT
  ; These fixture gates are compiled OUT of every distributed Setup.
  ${If} ${FileExists} "${QA_ROOT}\hold-lock"
    FileOpen $0 "${QA_ROOT}\lock-held" w
    FileClose $0
    StrCpy $1 0
    ${DoWhile} ${FileExists} "${QA_ROOT}\hold-lock"
      Sleep 100
      IntOp $1 $1 + 1
      ${If} $1 > 300
        Goto initialization_failed
      ${EndIf}
    ${Loop}
  ${EndIf}
!endif
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=inspector.exe "${PAYLOAD}"
  ClearErrors
  GetTempFileName $Stage "$LocalBase"
  IfErrors initialization_failed
  Delete "$Stage"
  StrCpy $Stage "$Stage.recovery"
!ifdef QA_ROOT
  StrCpy $QA_JournalWrites 0
  ${If} ${FileExists} "${QA_ROOT}\force-stage-collision"
    StrCpy $Stage "${QA_ROOT}\stale.recovery"
  ${EndIf}
!endif
  ; Reserve the actual recovery directory exclusively. Existing candidates,
  ; including private old evidence, are never adopted or repaired. The owner-
  ; only protected inheritable DACL applies at the first filesystem operation.
  StrCpy $3 "O:$OwnerSID"
  StrCpy $3 "$3D:P(A;OICI;FA;;;$OwnerSID)"
  System::Call 'advapi32::ConvertStringSecurityDescriptorToSecurityDescriptorW(w r3, i 1, *p.r0, p 0) i.r1'
  StrCmp $1 0 initialization_failed
  System::Call '*(&l4, p r0, i 0) p.r2'
  ${If} $2 == 0
    System::Call 'kernel32::LocalFree(p r0)'
    Goto initialization_failed
  ${EndIf}
  System::Call 'kernel32::CreateDirectoryW(w "$Stage", p r2) i.r1'
  System::Free $2
  System::Call 'kernel32::LocalFree(p r0)'
  StrCmp $1 0 initialization_failed
  Return
initialization_failed:
  SetErrorLevel 1
  Quit
FunctionEnd
!macroend
!insertmacro Initialize ""
!insertmacro Initialize "un"

Section "Install"
  !insertmacro ResetFileState exe
  !insertmacro ResetFileState uninstaller
  !insertmacro ResetFileState license
  !insertmacro ResetFileState notices
  !insertmacro ResetFileState release
  !insertmacro ResetFileState launch
  !insertmacro ResetFileState settings
  StrCpy $RegistryTouched 0
  StrCpy $RollbackError 0
  StrCpy $WasRunning 0
  StrCpy $LedgerMoved 0
  StrCpy $LedgerCommitted 0
  !insertmacro Inspect preflight-install preflight_failed
  !insertmacro StopOwned preflight_failed
  ; Check again after the verified process handle has signaled exit.
  !insertmacro Inspect preflight-install preflight_failed
  ClearErrors
  CopyFiles /SILENT "$Stage\report.ini" "$Stage\before.ini"
  IfErrors preflight_failed
  !insertmacro BackupValue display-name DisplayName $OldName
  !insertmacro BackupValue display-version DisplayVersion $OldVersion
  !insertmacro BackupValue publisher Publisher $OldPublisher
  !insertmacro BackupValue uninstall-command UninstallString $OldUninstall
  !insertmacro BackupValue install-location InstallLocation $OldLocation
  SetOutPath "$Stage"
  ClearErrors
  CopyFiles /SILENT "$PLUGINSDIR\inspector.exe" "$Stage\exe.new"
  IfErrors install_failed
  File /oname=license.new "${REPO}\LICENSE"
  File /oname=notices.new "${REPO}\THIRD_PARTY_NOTICES.txt"
  File /oname=release.new "${METADATA}"
  WriteUninstaller "$Stage\uninstaller.new"
  IfErrors install_failed
  ClearErrors
  CreateDirectory "$InstallDir"
  CreateDirectory "$MenuDir"
  IfErrors install_failed
  SetOutPath "$InstallDir"
  ClearErrors
  CreateShortcut "$Stage\launch.new" "$InstallDir\steam-sage-companion.exe" 'launch --config-dir "$ConfigDir"'
  CreateShortcut "$Stage\settings.new" "$InstallDir\steam-sage-companion.exe" 'settings --config-dir "$ConfigDir"'
  IfErrors install_failed
  SetOutPath "$Stage"
  !insertmacro ReplaceFile exe "$InstallDir\steam-sage-companion.exe"
  !insertmacro ReplaceFile uninstaller "$InstallDir\Uninstall.exe"
  !insertmacro ReplaceFile license "$InstallDir\LICENSE"
  !insertmacro ReplaceFile notices "$InstallDir\THIRD_PARTY_NOTICES.txt"
  !insertmacro ReplaceFile release "$InstallDir\release.json"
  !insertmacro ReplaceFile launch "$MenuDir\Steam Sage Companion.lnk"
  !insertmacro ReplaceFile settings "$MenuDir\Companion Settings.lnk"
  StrCpy $RegistryTouched 1
  !insertmacro WriteValue DisplayName "Steam Sage Companion"
  !insertmacro WriteValue DisplayVersion "${VERSION}"
  !insertmacro WriteValue Publisher "Effigy Analytics"
  !insertmacro WriteValue UninstallString '$\"$InstallDir\Uninstall.exe$\"'
  !insertmacro WriteValue InstallLocation "$InstallDir"
!ifdef QA_ROOT
  ${If} ${FileExists} "${QA_ROOT}\fail-after-registration"
    Goto install_failed
  ${EndIf}
!endif
  ; The candidate hashes actual final bytes and exact registration; ledger last.
  !insertmacro Inspect snapshot install_failed
  ${If} ${FileExists} "$InstallDir\package.ini"
    ClearErrors
    Rename "$InstallDir\package.ini" "$Stage\ledger.old"
    IfErrors install_failed
    StrCpy $LedgerMoved 1
  ${EndIf}
  ClearErrors
  Rename "$Stage\candidate.ini" "$InstallDir\package.ini"
  IfErrors install_failed
  StrCpy $LedgerCommitted 1
  SetOutPath "$TEMP"
  !insertmacro CleanStage
!ifndef QA_ROOT
  IfSilent launch_silent launch_interactive
launch_silent:
  ExecWait '"$InstallDir\steam-sage-companion.exe" launch --background --config-dir "$ConfigDir"' $0
  Goto launched
launch_interactive:
  ExecWait '"$InstallDir\steam-sage-companion.exe" launch --config-dir "$ConfigDir"' $0
launched:
  ${If} $0 != 0
    DetailPrint "Files installed. The companion could not start; open Companion Settings to retry."
    SetErrorLevel 2
    Goto install_done
  ${EndIf}
!endif
  DetailPrint "Installed per user. Start at login remains controlled by Companion Settings."
  SetErrorLevel 0
  Goto install_done

install_failed:
  ; Restore only destinations this transaction just replaced. Evidence persists
  ; for ordinary failures, whether restoration succeeds or needs manual review.
  !insertmacro RestoreFile settings "$MenuDir\Companion Settings.lnk"
  !insertmacro RestoreFile launch "$MenuDir\Steam Sage Companion.lnk"
  !insertmacro RestoreFile release "$InstallDir\release.json"
  !insertmacro RestoreFile notices "$InstallDir\THIRD_PARTY_NOTICES.txt"
  !insertmacro RestoreFile license "$InstallDir\LICENSE"
  !insertmacro RestoreFile uninstaller "$InstallDir\Uninstall.exe"
  !insertmacro RestoreFile exe "$InstallDir\steam-sage-companion.exe"
  ${If} $RegistryTouched == 1
    !insertmacro RestoreValue display-name DisplayName $OldName
    !insertmacro RestoreValue display-version DisplayVersion $OldVersion
    !insertmacro RestoreValue publisher Publisher $OldPublisher
    !insertmacro RestoreValue uninstall-command UninstallString $OldUninstall
    !insertmacro RestoreValue install-location InstallLocation $OldLocation
    DeleteRegKey /ifempty HKCU "${UNINSTALL_KEY}"
  ${EndIf}
  ${If} $LedgerCommitted == 1
    Delete "$InstallDir\package.ini"
  ${EndIf}
  ${If} $LedgerMoved == 1
    ClearErrors
    Rename "$Stage\ledger.old" "$InstallDir\package.ini"
    ${If} ${Errors}
      StrCpy $RollbackError 1
    ${EndIf}
  ${EndIf}
  ${If} $RollbackError == 0
  ${AndIf} $WasRunning == 1
!ifndef QA_ROOT
    ExecWait '"$InstallDir\steam-sage-companion.exe" launch --background --config-dir "$ConfigDir"' $0
!endif
  ${EndIf}
  DetailPrint "Installation failed. Previous verified entries were restored where possible. Recovery evidence: $Stage"
  Goto package_error
preflight_failed:
  DetailPrint "Installation preserved existing entries. Close the companion or review changed/unowned files. Report: $Stage"
package_error:
  IfSilent +2
    MessageBox MB_OK|MB_ICONEXCLAMATION "Setup could not finish. Existing or changed entries were preserved. Recovery information: $Stage"
  SetErrorLevel 1
install_done:
SectionEnd

Section "Uninstall"
  StrCpy $WasRunning 0
  !insertmacro Inspect preflight-uninstall uninstall_failed
  ReadINIStr $Leftovers "$Stage\report.ini" "package" "leftovers"
  !insertmacro StopOwned uninstall_failed
  !insertmacro Inspect preflight-uninstall uninstall_failed
  ReadINIStr $Leftovers "$Stage\report.ini" "package" "leftovers"
  ; A retained, verified ledger allows cleanup retries after ordinary errors.
  ${If} ${FileExists} "$InstallDir\package.ini"
    ClearErrors
    CopyFiles /SILENT "$InstallDir\package.ini" "$Stage\ledger.old"
    IfErrors uninstall_failed
  ${EndIf}
  SetOutPath "$TEMP"
  !insertmacro RemoveOwnedFile exe "$InstallDir\steam-sage-companion.exe"
  !insertmacro RemoveOwnedFile license "$InstallDir\LICENSE"
  !insertmacro RemoveOwnedFile notices "$InstallDir\THIRD_PARTY_NOTICES.txt"
  !insertmacro RemoveOwnedFile release "$InstallDir\release.json"
  !insertmacro RemoveOwnedFile launch "$MenuDir\Steam Sage Companion.lnk"
  !insertmacro RemoveOwnedFile settings "$MenuDir\Companion Settings.lnk"
  !insertmacro RemoveOwnedValue run "${RUN_KEY}" SteamSageCompanion
  ${If} $Leftovers == "no"
    !insertmacro RemoveOwnedValue display-name "${UNINSTALL_KEY}" DisplayName
    !insertmacro RemoveOwnedValue display-version "${UNINSTALL_KEY}" DisplayVersion
    !insertmacro RemoveOwnedValue publisher "${UNINSTALL_KEY}" Publisher
    !insertmacro RemoveOwnedValue uninstall-command "${UNINSTALL_KEY}" UninstallString
    !insertmacro RemoveOwnedValue install-location "${UNINSTALL_KEY}" InstallLocation
    DeleteRegKey /ifempty HKCU "${UNINSTALL_KEY}"
  ${EndIf}
  RMDir "$MenuDir"
  ${If} $Leftovers == "no"
    ClearErrors
    Delete "$InstallDir\package.ini"
    ${If} ${Errors}
      StrCpy $Leftovers "yes"
    ${Else}
      !insertmacro RemoveOwnedFile uninstaller "$InstallDir\Uninstall.exe"
      ${If} $Leftovers == "yes"
        CopyFiles /SILENT "$Stage\ledger.old" "$InstallDir\package.ini"
      ${EndIf}
    ${EndIf}
  ${EndIf}
  ${If} $Leftovers == "yes"
    DetailPrint "Changed/unowned or locked entries were kept, together with cleanup evidence. Report: $Stage"
    SetErrorLevel 2
  ${Else}
    RMDir "$InstallDir"
    !insertmacro CleanStage
    DetailPrint "Uninstalled. Companion settings, Steam files and browser choices were preserved."
    SetErrorLevel 0
  ${EndIf}
  Goto uninstall_done
uninstall_failed:
  DetailPrint "Uninstall preserved entries because ownership or process exit could not be verified. Report: $Stage"
  IfSilent +2
    MessageBox MB_OK|MB_ICONEXCLAMATION "Uninstall could not safely finish. Entries and settings were preserved. Review: $Stage"
  SetErrorLevel 1
uninstall_done:
SectionEnd
