import { Box, Button } from "@mui/material";
import type { Ref } from "react";
import { useImperativeHandle, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import rehypeRaw from "rehype-raw";

import { BaseDialog, DialogRef } from "@/components/base";
import { useUpdate } from "@/hooks/use-update";
import { openWebUrl } from "@/services/cmds";

export function UpdateViewer({ ref }: { ref?: Ref<DialogRef> }) {
  const { t } = useTranslation();

  const [open, setOpen] = useState(false);
  const { updateInfo } = useUpdate();

  useImperativeHandle(ref, () => ({
    open: () => setOpen(true),
    close: () => setOpen(false),
  }));

  const markdownContent = useMemo(() => {
    if (!updateInfo?.body) {
      return "New Version is available";
    }
    return updateInfo?.body;
  }, [updateInfo]);

  const onGoToRelease = () => {
    if (updateInfo?.downloadUrl) {
      openWebUrl(updateInfo.downloadUrl);
    } else {
      openWebUrl("https://github.com/liuguangzhong/clashgo/releases");
    }
  };

  return (
    <BaseDialog
      open={open}
      title={
        <Box display="flex" justifyContent="space-between">
          {t("settings.modals.update.title", {
            version: updateInfo?.version ?? "",
          })}
          <Box>
            <Button
              variant="contained"
              size="small"
              onClick={onGoToRelease}
            >
              {t("settings.modals.update.actions.goToRelease")}
            </Button>
          </Box>
        </Box>
      }
      contentSx={{ minWidth: 360, maxWidth: 400, height: "50vh" }}
      okBtn={t("settings.modals.update.actions.goToRelease")}
      cancelBtn={t("shared.actions.cancel")}
      onClose={() => setOpen(false)}
      onCancel={() => setOpen(false)}
      onOk={onGoToRelease}
    >
      <Box sx={{ height: "calc(100% - 10px)", overflow: "auto" }}>
        <ReactMarkdown
          rehypePlugins={[rehypeRaw]}
          components={{
            a: ({ ...props }) => {
              const { children } = props;
              return (
                <a {...props} target="_blank">
                  {children}
                </a>
              );
            },
          }}
        >
          {markdownContent}
        </ReactMarkdown>
      </Box>
    </BaseDialog>
  );
}
