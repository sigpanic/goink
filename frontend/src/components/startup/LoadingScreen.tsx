import { useTranslation } from "react-i18next";

/** 启动期间的占位屏：拿到快照但还不能确定渲染哪个界面的那段时间。 */
export default function LoadingScreen() {
  const { t } = useTranslation();

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <p className="text-muted-foreground">{t("app.loading")}</p>
    </div>
  );
}
