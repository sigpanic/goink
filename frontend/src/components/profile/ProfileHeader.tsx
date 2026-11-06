import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { User, Camera } from "lucide-react";
import { toErrorMessage } from "@/utils/error";
import { useSaveAvatar } from "./useSaveAvatar";
import { useSaveUserName } from "./useSaveUserName";

export default function ProfileHeader({
  userName,
  activityDays,
}: {
  userName: string;
  activityDays: number;
}) {
  const { t } = useTranslation();
  // 5.7 commit 2: SaveAvatar/SaveUserName 改 mutation（删 useApp）。
  // invalidate settingsKeys.all 由 useSaveUserName onSuccess 接管。
  const saveAvatar = useSaveAvatar();
  const saveUserName = useSaveUserName();

  // 组件内 UI state（规则 10，不进 store）
  const [avatarKey, setAvatarKey] = useState(0);
  const [editingName, setEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [avatarErrored, setAvatarErrored] = useState(false);
  const [avatarError, setAvatarError] = useState("");
  const [nameError, setNameError] = useState("");
  const fileInputRef = useRef<HTMLInputElement>(null);

  function handleAvatarClick() {
    fileInputRef.current?.click();
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = "";
    try {
      const buf = await file.arrayBuffer();
      await saveAvatar.mutateAsync(Array.from(new Uint8Array(buf)));
      setAvatarErrored(false);
      setAvatarKey((prev) => prev + 1);
      setAvatarError("");
    } catch (err) {
      setAvatarError(toErrorMessage(err));
    }
  }

  async function handleNameSave() {
    const name = nameDraft.trim();
    if (name && name !== userName) {
      try {
        await saveUserName.mutateAsync(name);
        // invalidate settingsKeys.all 由 useSaveUserName onSuccess 接管
        // （useProfileSettings 自动 refetch 拿新 user_name）。
        setNameError("");
      } catch (err) {
        setNameError(toErrorMessage(err));
        return;
      }
    }
    setEditingName(false);
  }

  function startEditName() {
    setNameDraft(userName);
    setEditingName(true);
  }

  return (
    <>
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={handleFileChange}
      />
      {/* 头像 + 问候 */}
      <div className="flex items-center gap-4">
        <div
          className="relative group flex-shrink-0 cursor-pointer select-none"
          onClick={handleAvatarClick}
        >
          {avatarErrored ? (
            <div className="w-14 h-14 rounded-full bg-muted bg-secondary flex items-center justify-center">
              <User className="w-7 h-7 text-muted-foreground" />
            </div>
          ) : (
            <img
              src={`/avatar?v=${avatarKey}`}
              alt=""
              onError={() => setAvatarErrored(true)}
              className="w-14 h-14 rounded-full object-cover"
            />
          )}
          <div className="absolute inset-0 rounded-full flex items-center justify-center bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity">
            <Camera className="w-5 h-5 text-white" />
          </div>
        </div>
        {avatarError && (
          <p className="text-xs text-destructive">{avatarError}</p>
        )}
        <div>
          {editingName ? (
            <div>
              <input
                autoFocus
                value={nameDraft}
                onChange={(e) => {
                  setNameDraft(e.target.value);
                  setNameError("");
                }}
                onBlur={handleNameSave}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleNameSave();
                  if (e.key === "Escape") setEditingName(false);
                }}
                className="text-lg font-semibold bg-transparent border-b border-primary outline-none text-foreground max-w-[200px]"
              />
              {nameError && (
                <p className="text-xs text-destructive mt-0.5">{nameError}</p>
              )}
            </div>
          ) : (
            <h1
              onClick={startEditName}
              className={`text-lg font-semibold cursor-pointer hover:text-primary transition-colors select-none ${userName ? "text-foreground" : "text-muted-foreground"}`}
            >
              {userName || t("profile.noNickname")}
            </h1>
          )}
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("profile.pastYearStats", { count: activityDays })}
          </p>
        </div>
      </div>
    </>
  );
}
