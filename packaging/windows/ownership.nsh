; All mutation destinations are compile-time fixed IDs. Report text supplies
; statuses only; ledger paths and registry data are never executable authority.
!macro Inspect OP FAILURE
  ClearErrors
  ExecWait '"$PLUGINSDIR\inspector.exe" package-check --operation ${OP} --stage-dir "$Stage" --report "$Stage\report.ini"' $0
  IfErrors ${FAILURE}
  StrCmp $0 0 +2
    Goto ${FAILURE}
  ReadINIStr $0 "$Stage\report.ini" "package" "schema"
  StrCmp $0 "1" +2
    Goto ${FAILURE}
  ReadINIStr $0 "$Stage\report.ini" "package" "result"
  StrCmp $0 "ok" +2
    Goto ${FAILURE}
  ; Cross-check independently resolved roots; never adopt a report/ledger path.
  ReadINIStr $0 "$Stage\report.ini" "package" "install"
  StrCmp $0 $InstallDir +2
    Goto ${FAILURE}
  ReadINIStr $0 "$Stage\report.ini" "package" "menu"
  StrCmp $0 $MenuDir +2
    Goto ${FAILURE}
  ReadINIStr $0 "$Stage\report.ini" "package" "config"
  StrCmp $0 $ConfigDir +2
    Goto ${FAILURE}
  ReadINIStr $0 "$Stage\report.ini" "package" "owner"
  StrCmp $0 $OwnerSID +2
    Goto ${FAILURE}
!macroend

!macro StopOwned FAILURE
  StrCpy $WasRunning 0
  ReadINIStr $0 "$Stage\report.ini" "exe" "status"
  ${If} $0 == "owned"
    ClearErrors
    ExecWait '"$PLUGINSDIR\inspector.exe" stop --config-dir "$ConfigDir"' $0
    IfErrors ${FAILURE}
    ${If} $0 == 0
      StrCpy $WasRunning 1
    ${ElseIf} $0 != 3
      Goto ${FAILURE}
    ${EndIf}
  ${EndIf}
!macroend

; QA-only sharing failures exercise actual WriteINIStr errors, without filling
; disks or changing permissions outside this fixture's private transaction.
!macro WriteJournal ID STATE
!ifdef QA_ROOT
  IntOp $QA_JournalWrites $QA_JournalWrites + 1
  StrCpy $8 0
  ${If} ${FileExists} "${QA_ROOT}\fail-journal-first"
  ${AndIf} $QA_JournalWrites == 1
    StrCpy $8 1
  ${EndIf}
  ${If} ${FileExists} "${QA_ROOT}\fail-journal-later"
  ${AndIf} $QA_JournalWrites == 5
    StrCpy $8 1
  ${EndIf}
  ${If} $8 == 1
    FileOpen $8 "$Stage\transaction.ini" a
    FileClose $8
    System::Call 'kernel32::CreateFileW(w "$Stage\transaction.ini", i 0x80000000, i 1, p 0, i 3, i 0x80, p 0) p.r8'
    StrCpy $QA_JournalLock $8
  ${EndIf}
!endif
  WriteINIStr "$Stage\transaction.ini" "${ID}" "${STATE}" "yes"
!ifdef QA_ROOT
  ${If} ${Errors}
    ${IfNot} ${FileExists} "${QA_ROOT}\journal-failure-observed"
      FileOpen $8 "${QA_ROOT}\journal-failure-observed" w
      FileWrite $8 "$QA_JournalWrites"
      FileClose $8
    ${EndIf}
    SetErrors
  ${EndIf}
!endif
!macroend

!macro DeclareFileState ID
  Var Moved_${ID}
  Var Created_${ID}
!macroend
!macro ResetFileState ID
  StrCpy $Moved_${ID} 0
  StrCpy $Created_${ID} 0
!macroend

!macro ReplaceFile ID DEST
  ReadINIStr $0 "$Stage\before.ini" "${ID}" "status"
  ${If} $0 == "owned"
    ClearErrors
    Rename "${DEST}" "$Stage\${ID}.old"
    IfErrors install_failed
    ; Memory is the rollback authority. A failed journal must not strand the
    ; backup that Rename just created.
    StrCpy $Moved_${ID} 1
    ClearErrors
    !insertmacro WriteJournal ${ID} old
    IfErrors install_failed
  ${ElseIf} $0 != "missing"
    Goto install_failed
  ${EndIf}
  ClearErrors
  !insertmacro WriteJournal ${ID} new
  IfErrors install_failed
  ; CopyFiles can leave a prefix even on failure; record intent in memory first.
  StrCpy $Created_${ID} 1
  ClearErrors
  CopyFiles /SILENT "$Stage\${ID}.new" "${DEST}"
  IfErrors install_failed
!macroend

!macro RestoreFile ID DEST
  ${If} $Created_${ID} == 1
    ClearErrors
    Delete "${DEST}"
    ${If} ${Errors}
      StrCpy $RollbackError 1
    ${EndIf}
  ${EndIf}
  ${If} $Moved_${ID} == 1
    ClearErrors
    Rename "$Stage\${ID}.old" "${DEST}"
    ${If} ${Errors}
      StrCpy $RollbackError 1
    ${EndIf}
  ${EndIf}
!macroend

!macro BackupValue ID NAME VAR
  ReadINIStr $0 "$Stage\before.ini" "${ID}" "status"
  ${If} $0 == "owned"
    ClearErrors
    ReadRegStr ${VAR} HKCU "${UNINSTALL_KEY}" "${NAME}"
    IfErrors install_failed
  ${ElseIf} $0 != "missing"
    Goto install_failed
  ${EndIf}
!macroend
!macro RestoreValue ID NAME VAR
  ReadINIStr $0 "$Stage\before.ini" "${ID}" "status"
  ClearErrors
  ${If} $0 == "owned"
    WriteRegStr HKCU "${UNINSTALL_KEY}" "${NAME}" ${VAR}
  ${ElseIf} $0 == "missing"
    DeleteRegValue HKCU "${UNINSTALL_KEY}" "${NAME}"
    ; A missing value is already the desired rollback state.
    ClearErrors
  ${EndIf}
  ${If} ${Errors}
    StrCpy $RollbackError 1
  ${EndIf}
!macroend
!macro WriteValue NAME VALUE
  ClearErrors
  WriteRegStr HKCU "${UNINSTALL_KEY}" "${NAME}" "${VALUE}"
  IfErrors install_failed
!macroend

!macro RemoveOwnedFile ID DEST
  ReadINIStr $0 "$Stage\report.ini" "${ID}" "status"
  ${If} $0 == "owned"
    ClearErrors
    Delete "${DEST}"
    ${If} ${Errors}
      StrCpy $Leftovers "yes"
    ${EndIf}
  ${ElseIf} $0 != "missing"
    StrCpy $Leftovers "yes"
  ${EndIf}
!macroend
!macro RemoveOwnedValue ID KEY NAME
  ReadINIStr $0 "$Stage\report.ini" "${ID}" "status"
  ${If} $0 == "owned"
    ClearErrors
    DeleteRegValue HKCU "${KEY}" "${NAME}"
    ${If} ${Errors}
      StrCpy $Leftovers "yes"
    ${EndIf}
  ${ElseIf} $0 != "missing"
    StrCpy $Leftovers "yes"
  ${EndIf}
!macroend

!macro CleanStageFile ID
  Delete "$Stage\${ID}.old"
  Delete "$Stage\${ID}.new"
!macroend
!macro CleanStage
  !insertmacro CleanStageFile exe
  !insertmacro CleanStageFile uninstaller
  !insertmacro CleanStageFile license
  !insertmacro CleanStageFile notices
  !insertmacro CleanStageFile release
  !insertmacro CleanStageFile launch
  !insertmacro CleanStageFile settings
  Delete "$Stage\ledger.old"
  Delete "$Stage\report.ini"
  Delete "$Stage\before.ini"
  Delete "$Stage\candidate.ini"
  Delete "$Stage\transaction.ini"
  RMDir "$Stage"
!macroend
