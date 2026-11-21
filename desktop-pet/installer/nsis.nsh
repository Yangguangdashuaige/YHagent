; electron-builder NSIS hooks. The app keeps every file under its install directory, its data in
; $INSTDIR\data, so the default location stays out of AppData and uninstalling keeps data\.

!ifdef APP_ARM64
  !ifndef APP_64
    ; makensis does not find an !include path written with forward slashes
    !include /CHARSET=UTF8 "${__FILEDIR__}\i18n\arm64.nsh"
  !endif
!endif

!macro checkExtracted
  ; the installer template never checks whether Nsis7z::Extract succeeded: an extraction that
  ; antivirus software or a full disk emptied read as success, and the wizard registers, shortcuts to
  ; and "runs" an install that wrote no files or only part of them. YHagent-Pet.exe or the TypeScript
  ; loader the Core starts with (app/core-host.cjs) missing is that case: stop here, before
  ; the uninstaller, the registry entry and the shortcuts are written, so a failed install leaves
  ; nothing behind.
  ${IfNot} ${FileExists} "$INSTDIR\YHagent-Pet.exe"
  ${OrIfNot} ${FileExists} "$INSTDIR\resources\app\node_modules\tsx\dist\loader.mjs"
    ; /SD: a silent install (an automatic update runs one) must not wait on the box
    MessageBox MB_OK|MB_ICONSTOP "The installer could not extract all ${PRODUCT_NAME} files. Check available disk space and run the installer again." /SD IDOK
    Abort
  ${EndIf}
!macroend

; customFiles_<architecture of the package> is undocumented but the first hook after the files land
; (customInstall runs only after the install is already registered)
!macro customFiles_x64
  !insertmacro checkExtracted
!macroend

!macro customFiles_arm64
  !insertmacro checkExtracted
!macroend

!macro customInstallMode
  ; the app writes data\ beside itself, which an install for all users (Program Files) cannot: the
  ; "for all users / only for me" page is skipped and the install is per-user, as "only for me" makes it
  StrCpy $isForceCurrentInstall "1"
!macroend

!macro customInit
  ; the Windows on Arm installer holds no files for another processor: there the template extracts
  ; nothing, runs no customFiles hook and registers the empty install
  !ifdef APP_ARM64
    !ifndef APP_64
      ${IfNot} ${IsNativeARM64}
        MessageBox MB_OK|MB_ICONSTOP "$(arm64Required)" /SD IDOK
        Quit
      ${EndIf}
    !endif
  !endif
  ; the per-user default is %LOCALAPPDATA%\Programs. An update stays where the install is, since data\ sits
  ; beside the program (app/main.cjs takes back a data\ that an earlier update left in the old place)
  StrLen $0 "$LOCALAPPDATA"
  StrCpy $1 "$INSTDIR" $0
  StrLen $2 "$APPDATA"
  StrCpy $3 "$INSTDIR" $2
  ${IfNot} ${isUpdated}
    ${If} $1 == "$LOCALAPPDATA"
    ${OrIf} $3 == "$APPDATA"
      StrCpy $INSTDIR "$PROFILE\YHagent-Pet"
    ${EndIf}
  ${EndIf}
!macroend

!macro customRemoveFiles
  ; what an Electron build installs, by name, so data\ and anything else put in the directory
  ; stay (an update runs this too)
  SetOutPath $TEMP
  RMDir /r "$INSTDIR\resources"
  RMDir /r "$INSTDIR\locales"
  Delete "$INSTDIR\YHagent-Pet.exe"
  Delete "$INSTDIR\Uninstall YHagent-Pet.exe"
  Delete "$INSTDIR\*.dll"
  Delete "$INSTDIR\*.pak"
  Delete "$INSTDIR\*.bin"
  Delete "$INSTDIR\*.dat"
  Delete "$INSTDIR\vk_swiftshader_icd.json"
  Delete "$INSTDIR\LICENSE.electron.txt"
  Delete "$INSTDIR\LICENSES.chromium.html"
  ; removed only when nothing is left, data\ included
  RMDir "$INSTDIR"
!macroend
