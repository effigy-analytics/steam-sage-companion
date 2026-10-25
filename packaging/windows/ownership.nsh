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

!macro ReplaceFile ID DEST
  ReadINIStr $0 "$Stage\before.ini" "${ID}" "status"
  ${If} $0 == "owned"
    ClearErrors
    Rename "${DEST}" "$Stage\${ID}.old"
    IfErrors install_failed
    WriteINIStr "$Stage\transaction.ini" "${ID}" "old" "yes"
  ${ElseIf} $0 != "missing"
    Goto install_failed
  ${EndIf}
  ; Mark the intended creation before CopyFiles, whose failure may leave a prefix.
  WriteINIStr "$Stage\transaction.ini" "${ID}" "new" "yes"
  ClearErrors
  CopyFiles /SILENT "$Stage\${ID}.new" "${DEST}"
  IfErrors install_failed
!macroend

!macro RestoreFile ID DEST
  ReadINIStr $0 "$Stage\transaction.ini" "${ID}" "new"
  ${If} $0 == "yes"
    ClearErrors
    Delete "${DEST}"
    ${If} ${Errors}
      StrCpy $RollbackError 1
    ${EndIf}
  ${EndIf}
  ReadINIStr $0 "$Stage\transaction.ini" "${ID}" "old"
  ${If} $0 == "yes"
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
