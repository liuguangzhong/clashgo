import { alpha, Box, styled } from "@mui/material";

export const ProfileBox = styled(Box)(({
  theme,
  "aria-selected": selected,
}) => {
  const { mode, primary, text } = theme.palette;
  const key = `${mode}-${!!selected}`;

  const backgroundColor = {
    "light-true": alpha(primary.main, 0.08),
    "light-false": "#ffffff",
    "dark-true": alpha(primary.main, 0.15),
    "dark-false": "#282A36",
  }[key]!;

  const color = {
    "light-true": text.secondary,
    "light-false": text.secondary,
    "dark-true": alpha(text.secondary, 0.75),
    "dark-false": alpha(text.secondary, 0.65),
  }[key]!;

  const h2color = {
    "light-true": primary.main,
    "light-false": text.primary,
    "dark-true": primary.main,
    "dark-false": text.primary,
  }[key]!;

  const borderSelect = {
    "light-true": {
      borderLeft: `4px solid ${primary.main}`,
      width: `calc(100% + 4px)`,
      marginLeft: `-4px`,
    },
    "light-false": {
      borderLeft: `4px solid transparent`,
      width: "100%",
    },
    "dark-true": {
      borderLeft: `4px solid ${primary.main}`,
      width: `calc(100% + 4px)`,
      marginLeft: `-4px`,
    },
    "dark-false": {
      borderLeft: `4px solid transparent`,
      width: "100%",
    },
  }[key];

  const boxShadow = selected
    ? mode === "light"
      ? `0 2px 8px ${alpha(primary.main, 0.25)}`
      : `0 2px 12px ${alpha(primary.main, 0.35)}`
    : "none";

  return {
    position: "relative",
    display: "block",
    cursor: "pointer",
    textAlign: "left",
    padding: "8px 16px",
    boxSizing: "border-box",
    backgroundColor,
    ...borderSelect,
    borderRadius: "8px",
    boxShadow,
    color,
    transition: "all 0.2s ease-in-out",
    "& h2": { color: h2color },
    "&:hover": {
      boxShadow: selected
        ? boxShadow
        : mode === "light"
          ? "0 2px 6px rgba(0,0,0,0.1)"
          : "0 2px 6px rgba(0,0,0,0.3)",
    },
  };
});

