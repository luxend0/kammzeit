# Requisiten im Toon-Look: Salonstuhl, Spiegelplatz, Pflanze, Foehn, Handy mit Haken, Schere, Kamm.
# Alle Generatoren: jedes yield = ein sichtbarer Bauschritt. Ursprung = Bodenmitte, Front nach -Y.
import math
from kzlib import (toon, flat, empty, slab, ring_slab, ellipse, rrect, loft_z, lathe, capsule, ellipsoid,
                   band, bez2, text, shadow, log, mix_hex,
                   SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE)

R = math.radians
SILVER = mix_hex(WHITE, NIGHT, 0.14)


def salon_chair(n, root, seat=BLUE, metal=SILVER):
    mm = toon(metal)
    sm = toon(seat)
    lathe(n + "_Fuss", [(0.045, 0.0), (0.045, 0.27), (0.03, 0.3), (0.0, 0.3), (0.0, 0.0)], n=64, parent=root, mat=mm)
    yield
    lathe(n + "_Saeule", [(0.37, 0.0), (0.37, 0.07), (0.3, 0.07), (0.3, 0.045), (0.04, 0.045), (0.04, 0.0)],
          parent=root, mat=mm)
    yield
    loft_z(n + "_Sitz", [(0.36, 0.24, 0.22, 4), (0.39, 0.27, 0.25, 4), (0.47, 0.275, 0.255, 4),
                         (0.495, 0.25, 0.23, 4), (0.5, 0.0, 0.0, 4)], parent=root, mat=sm, loc=(0, 0.02, 0))
    log("Sitz")
    yield
    loft_z(n + "_Lehne", [(0.0, 0.2, 0.045, 4), (0.04, 0.235, 0.062, 4), (0.5, 0.24, 0.062, 4),
                          (0.56, 0.21, 0.05, 4), (0.57, 0.0, 0.0, 4)], parent=root, mat=sm,
           loc=(0, 0.25, 0.46), rot=(R(-8), 0, 0))
    log("Lehne")
    yield
    for sx in (-1, 1):
        capsule("%s_Stuetze_%d" % (n, sx), 0.16, 0.022, 0.022, parent=root, mat=mm, loc=(sx * 0.29, -0.05, 0.65))
        loft_z("%s_Armlehne_%d" % (n, sx), [(0.0, 0.0, 0.0, 4), (0.01, 0.04, 0.03, 4), (0.4, 0.045, 0.034, 4),
                                            (0.43, 0.035, 0.026, 4), (0.44, 0.0, 0.0, 4)],
               parent=root, mat=sm, loc=(sx * 0.29, 0.2, 0.67), rot=(R(90), 0, 0))
    log("Armlehnen")
    yield
    capsule(n + "_Fussstrebe", 0.33, 0.02, 0.02, parent=root, mat=mm, loc=(0, -0.04, 0.24), rot=(R(-74), 0, 0))
    capsule(n + "_Fussstuetze", 0.44, 0.022, 0.022, parent=root, mat=mm, loc=(0.22, -0.36, 0.15), rot=(0, R(90), 0))
    log("Fussstuetze")
    yield


def mirror_station(n, root, frame=YELLOW, cabinet=WHITE):
    cm = toon(cabinet)
    loft_z(n + "_Schrank", [(0.0, 0.43, 0.2, 6), (0.76, 0.43, 0.2, 6), (0.78, 0.0, 0.0, 6)], parent=root, mat=cm)
    yield
    slab(n + "_Platte", rrect(0.96, 0.46, 0.04, 6), 0.78, 0.82, parent=root, mat=toon(NIGHT, shade=0.3))
    for i, z in enumerate((0.52, 0.26)):
        slab("%s_Schublade_%d" % (n, i), rrect(0.76, 0.012, 0.004, 3), 0, 0.006, parent=root, mat=flat(mix_hex(cabinet, NIGHT, 0.18)),
             line=False, loc=(0, -0.2005, z), rot=(R(90), 0, 0))
        ellipsoid("%s_Knopf_%d" % (n, i), 0.03, 0.012, 0.012, parent=root, mat=toon(NIGHT), loc=(0, -0.205, z + 0.11))
    log("Schrank")
    yield
    m = empty(n + "_Spiegel", loc=(0, 0.1, 0.82), rot=(R(90), 0, 0), parent=root)
    ring_slab(n + "_Rahmen", rrect(0.7, 1.08, 0.3, 16, 0, 0.54), rrect(0.58, 0.96, 0.25, 16, 0, 0.54), 0, 0.045,
              parent=m, mat=toon(frame))
    log("Spiegelrahmen")
    yield
    slab(n + "_Glas", rrect(0.59, 0.97, 0.25, 16, 0, 0.54), 0.018, 0.022, parent=m, mat=flat(SKY_L), line=False)
    for i, (x0, w) in enumerate(((-0.17, 0.07), (-0.05, 0.03))):
        pts = [(x0, 0.2), (x0 + w, 0.2), (x0 + w + 0.24, 0.62), (x0 + 0.24, 0.62)]
        slab("%s_Glanz_%d" % (n, i), pts, 0.0225, 0.023, parent=m, mat=flat(WHITE), line=False)
    log("Spiegelglas")
    yield


def plant(n, root, pot=CORAL, leaf=MINT, height=1.0, leaves=7):
    lathe(n + "_Topf", [(0.4, 0.0), (0.4, 0.175), (0.37, 0.18), (0.35, 0.165), (0.0, 0.125), (0.0, 0.0)], n=56,
          parent=root, mat=toon(pot))
    slab(n + "_Erde", ellipse(0, 0, 0.162, 0.162, 48), 0.38, 0.385, parent=root, mat=flat(mix_hex(NIGHT, pot, 0.25)), line=False)
    log("Topf")
    yield
    lm = toon(leaf, shade=0.24)
    stem = toon(mix_hex(leaf, NIGHT, 0.35))
    for i in range(leaves):
        a = 360 * i / leaves + 18
        tilt = 28 + 14 * (i % 3)
        L = (0.35 + 0.18 * ((i * 7) % 5) / 4) * height
        st = empty("%s_Stiel_%d" % (n, i), loc=(0, 0, 0.36), rot=(R(tilt), 0, R(a)), parent=root)
        capsule("%s_Stielform_%d" % (n, i), L, 0.011, 0.009, parent=st, mat=stem, rot=(R(180), 0, 0))
        ellipsoid("%s_Blatt_%d" % (n, i), 0.1 * height, 0.014, 0.17 * height, parent=st, mat=lm,
                  loc=(0, 0, L + 0.12 * height))
        if i % 2 == 1:
            yield 0.15
    log("Blaetter")
    yield


def dryer(n, root, body=YELLOW, dark=NIGHT):
    """Foehn: lokal entlang +X gebaut, Griff nach -Z."""
    bm = toon(body)
    dm = toon(dark, shade=0.3)
    loft_z(n + "_Koerper", [(0.0, 0.0, 0.0, 2), (0.0, 0.06, 0.06, 2), (0.015, 0.072, 0.072, 2), (0.13, 0.072, 0.072, 2),
                            (0.17, 0.058, 0.058, 2)], parent=root, mat=bm, rot=(0, R(90), 0))
    loft_z(n + "_Duese", [(0.165, 0.058, 0.058, 2), (0.22, 0.045, 0.024, 2.8), (0.255, 0.046, 0.016, 3.2),
                          (0.256, 0.0, 0.0, 3)], parent=root, mat=dm, rot=(0, R(90), 0))
    slab(n + "_Gitter", ellipse(0, 0, 0.05, 0.05, 48), -0.004, 0.0, parent=root, mat=flat(mix_hex(dark, body, 0.2)),
         line=False, rot=(0, R(-90), 0))
    capsule(n + "_Griff", 0.17, 0.034, 0.03, parent=root, mat=bm, loc=(0.07, 0, -0.03), rot=(0, R(-12), 0))
    ellipsoid(n + "_Schalter", 0.014, 0.02, 0.03, parent=root, mat=toon(CORAL), loc=(0.085, -0.03, -0.09), rot=(0, R(-12), 0))
    capsule(n + "_Kabel", 0.12, 0.012, 0.012, parent=root, mat=dm, loc=(0.105, 0, -0.205), rot=(0, R(-40), 0))
    log("Foehn")
    yield


def phone(n, root, check=True, screen=WHITE):
    """Handy, Bildschirm zeigt nach -Y (zur Kamera), steht senkrecht in der lokalen XZ-Ebene."""
    p = empty(n, rot=(R(90), 0, 0), parent=root)
    slab(n + "_Gehaeuse", rrect(0.078, 0.158, 0.012, 8), -0.004, 0.004, parent=p, mat=toon(NIGHT, shade=0.3), bev=0.002, seg=2)
    slab(n + "_Display", rrect(0.068, 0.146, 0.008, 8), 0.004, 0.0045, parent=p, mat=flat(screen), line=False)
    if check:
        slab(n + "_Haken_Kreis", ellipse(0, 0.012, 0.024, 0.024, 48), 0.0046, 0.005, parent=p, mat=flat(MINT), line=False)
        pts = [(-0.011, 0.012), (-0.003, 0.004), (0.012, 0.021)]
        slab(n + "_Haken", band(pts, 0.006, 0.006, round_end=True, round_start=True), 0.0051, 0.0055, parent=p,
             mat=flat(WHITE), line=False)
        for i, w in enumerate((0.04, 0.028)):
            slab("%s_Zeile_%d" % (n, i), rrect(w, 0.006, 0.003, 4, 0, -0.03 - 0.013 * i), 0.0046, 0.005, parent=p,
                 mat=flat(mix_hex(WHITE, NIGHT, 0.18)), line=False)
    log("Handy")
    yield
    return p


def upright(n, parent, loc=(0, 0, 0), rz=0.0):
    """Gruppe, deren lokale XY-Ebene senkrecht steht (x -> x, y -> z), Vorderseite +z -> -Y."""
    return empty(n, loc=loc, rot=(R(90), 0, R(rz)), parent=parent)


def sofa(n, root, color=BLUE, legs=NIGHT, width=1.9):
    c = toon(color)
    hw = width / 2
    loft_z(n + "_Unterbau", [(0.11, hw - 0.02, 0.4, 6), (0.13, hw, 0.42, 6), (0.4, hw, 0.42, 6), (0.42, hw - 0.02, 0.4, 6)],
           parent=root, mat=c)
    for sx in (-1, 1):
        for sy in (-1, 1):
            lathe("%s_Fuss_%d_%d" % (n, sx, sy), [(0.12, 0.03), (0.0, 0.02), (0.0, 0.0)], n=16, parent=root,
                  mat=toon(legs), loc=(sx * (hw - 0.12), sy * 0.3, 0))
    log("Sofa Unterbau")
    yield
    loft_z(n + "_Lehne", [(0.38, hw - 0.04, 0.12, 6), (0.92, hw - 0.04, 0.12, 6), (0.96, hw - 0.1, 0.1, 5),
                          (0.97, 0.0, 0.0, 5)], parent=root, mat=c, loc=(0, 0.3, 0))
    log("Sofa Lehne")
    yield
    cw = (width - 0.36) / 2
    for i, sx in enumerate((-1, 1)):
        loft_z("%s_Kissen_%d" % (n, i), [(0.41, cw / 2 - 0.02, 0.33, 5), (0.44, cw / 2, 0.35, 5), (0.53, cw / 2, 0.35, 5),
                                         (0.56, cw / 2 - 0.03, 0.32, 5), (0.565, 0.0, 0.0, 5)],
               parent=root, mat=c, loc=(sx * cw / 2, -0.04, 0))
    log("Sitzkissen")
    yield
    for sx in (-1, 1):
        loft_z("%s_Armlehne_%d" % (n, sx), [(0.11, 0.1, 0.4, 6), (0.62, 0.11, 0.42, 6), (0.68, 0.09, 0.4, 5),
                                            (0.69, 0.0, 0.0, 5)], parent=root, mat=c, loc=(sx * (hw - 0.08), 0, 0))
    log("Armlehnen")
    yield


def pillow(n, root, color=YELLOW, size=0.42):
    loft_z(n, [(-0.06, size / 2 - 0.04, size / 2 - 0.04, 3.2), (0.0, size / 2, size / 2, 3.2),
               (0.06, size / 2 - 0.04, size / 2 - 0.04, 3.2)], parent=root, mat=toon(color), rot=(R(90), 0, 0))
    yield 0.2


def window_wall(n, root, wall=None, w=2.5, h=2.1, win_x=-0.5, moon=True, win_y=1.3, win_w=0.92, win_h=1.08,
                day=False):
    """Wandstueck (steht auf dem Boden) mit Fenster: nachts mit Mond und Sternen, tags mit Wolken."""
    wall = wall or mix_hex(SKY, WHITE, 0.5)
    g = upright(n, root)
    slab(n + "_Wand", rrect(w, h, 0.1, 8, 0, h / 2), 0, 0.06, parent=g, mat=toon(wall))
    log("Wand")
    yield
    cx, cy, ww, wh = win_x, win_y, win_w, win_h
    slab(n + "_Scheibe", rrect(ww - 0.06, wh - 0.06, 0.1, 10, cx, cy), 0.06, 0.07, parent=g,
         mat=flat(SKY if day else NIGHT), line=False)
    ring_slab(n + "_Fensterrahmen", rrect(ww, wh, 0.13, 10, cx, cy), rrect(ww - 0.1, wh - 0.1, 0.09, 10, cx, cy),
              0.06, 0.1, parent=g, mat=toon(WHITE))
    slab(n + "_Sprosse_V", rrect(0.04, wh - 0.08, 0.01, 2, cx, cy), 0.07, 0.09, parent=g, mat=toon(WHITE))
    slab(n + "_Sprosse_H", rrect(ww - 0.08, 0.04, 0.01, 2, cx, cy - 0.05), 0.07, 0.09, parent=g, mat=toon(WHITE))
    slab(n + "_Bank", rrect(ww + 0.14, 0.05, 0.015, 3, cx, cy - wh / 2 - 0.02), 0.06, 0.16, parent=g, mat=toon(WHITE))
    log("Fenster")
    yield
    if day:
        for k, (ox, oy, sc) in enumerate(((-0.22 * ww, 0.22 * wh, 1.0), (0.2 * ww, -0.12 * wh, 0.8))):
            for m, (dx, dy, r) in enumerate(((-0.07, 0.0, 0.05), (0.0, 0.03, 0.07), (0.08, 0.0, 0.05), (0.0, -0.02, 0.05))):
                slab("%s_Wolke_%d_%d" % (n, k, m), ellipse(cx + ox + dx * sc, cy + oy + dy * sc, r * sc, r * sc * 0.85, 32),
                     0.071, 0.072, parent=g, mat=flat(WHITE), line=False)
            slab("%s_Wolke_%d_Boden" % (n, k), rrect(0.24 * sc, 0.05 * sc, 0.025 * sc, 6, cx + ox, cy + oy - 0.035 * sc),
                 0.071, 0.072, parent=g, mat=flat(WHITE), line=False)
        log("Wolken")
        yield
    elif moon:
        mx, my = cx + 0.2, cy + 0.27
        slab(n + "_Mond", ellipse(mx, my, 0.1, 0.1, 64), 0.071, 0.072, parent=g, mat=flat(YELLOW), line=False)
        slab(n + "_Mond_Schatten", ellipse(mx + 0.05, my + 0.035, 0.085, 0.085, 64), 0.0722, 0.0724, parent=g,
             mat=flat(NIGHT), line=False)
        for i, (sx, sy, r) in enumerate(((-0.28, 0.33, 0.014), (-0.12, 0.42, 0.009), (-0.3, -0.12, 0.01),
                                         (0.25, -0.25, 0.012), (-0.08, -0.3, 0.008), (0.3, 0.05, 0.008))):
            slab("%s_Stern_%d" % (n, i), ellipse(cx + sx, cy + sy, r, r, 16), 0.071, 0.072, parent=g,
                 mat=flat(WHITE if i % 2 else YELLOW), line=False)
        log("Mond und Sterne")
        yield
    return g


def side_table(n, root, color=WHITE, top_h=0.56):
    m = toon(color)
    lathe(n + "_Platte", [(top_h, 0.0), (top_h, 0.21), (top_h - 0.04, 0.22), (top_h - 0.05, 0.0)], n=48, parent=root, mat=m)
    lathe(n + "_Bein", [(top_h - 0.04, 0.025), (0.03, 0.025), (0.03, 0.0)], n=16, parent=root, mat=toon(NIGHT))
    lathe(n + "_Fuss", [(0.03, 0.0), (0.03, 0.15), (0.0, 0.16), (0.0, 0.0)], n=40, parent=root, mat=toon(NIGHT))
    log("Beistelltisch")
    yield


def digital_clock(n, root, time_text="23:47", body=WHITE, screen=NIGHT, digits=YELLOW):
    """Wecker mit Digitalanzeige, steht auf einer Flaeche, Anzeige nach -Y."""
    loft_z(n + "_Gehaeuse", [(0.0, 0.16, 0.06, 5), (0.13, 0.16, 0.06, 5), (0.15, 0.14, 0.05, 4), (0.152, 0.0, 0.0, 4)],
           parent=root, mat=toon(body))
    g = upright(n + "_Front", root, loc=(0, -0.06, 0))
    slab(n + "_Anzeige", rrect(0.27, 0.095, 0.02, 6, 0, 0.073), 0.0, 0.004, parent=g, mat=flat(screen), line=False)
    text(n + "_Ziffern", time_text, 0.066, font="Roboto-Bold.ttf", align="CENTER", parent=g, loc=(0, 0.048, 0.0045),
         mat=flat(digits))
    log("Wecker", time_text)
    yield


def floor_lamp(n, root, shade=YELLOW, pole=NIGHT, height=1.55):
    lathe(n + "_Fuss", [(0.025, 0.0), (0.025, 0.16), (0.0, 0.17), (0.0, 0.0)], n=40, parent=root, mat=toon(pole))
    lathe(n + "_Stange", [(height - 0.2, 0.014), (0.02, 0.014), (0.02, 0.0)], n=12, parent=root, mat=toon(pole))
    lathe(n + "_Schirm", [(height, 0.0), (height, 0.12), (height - 0.28, 0.22), (height - 0.29, 0.0)], n=48,
          parent=root, mat=toon(shade))
    log("Stehlampe")
    yield


def scissors(n, hand, blade=SILVER, rings=CORAL, open_deg=22):
    """Kleine Friseurschere in der Hand: Klingen zeigen entlang -Z (Unterarm-Richtung)."""
    g = empty(n, loc=(0, -0.03, -0.05), rot=(R(90), 0, 0), parent=hand)
    bm = toon(blade, shade=0.3)
    for sx in (-1, 1):
        a = R(sx * open_deg / 2)
        tip = (math.sin(a) * 0.12, -0.03 - math.cos(a) * 0.12)
        side = (math.cos(a) * 0.009, math.sin(a) * 0.009)
        pts = [(-side[0], -0.03 - side[1]), (side[0], -0.03 + side[1]), tip]
        slab("%s_Klinge_%d" % (n, sx), pts, 0.0, 0.004, parent=g, mat=bm)
        ring_slab("%s_Ring_%d" % (n, sx), ellipse(sx * 0.016, 0.012, 0.017, 0.014, 32), ellipse(sx * 0.016, 0.012, 0.009, 0.007, 32),
                  0.0, 0.006, parent=g, mat=toon(rings))
        slab("%s_Steg_%d" % (n, sx), [(sx * 0.004, -0.03), (sx * 0.012, 0.0), (sx * 0.006, 0.0)], 0.0, 0.005, parent=g, mat=bm)
    slab(n + "_Schraube", ellipse(0, -0.03, 0.006, 0.006, 16), 0.004, 0.007, parent=g, mat=toon(NIGHT))
    log("Schere")
    yield 0.2


def comb(n, hand, color=NIGHT):
    """Kamm in der Hand, Zaehne nach -X."""
    g = empty(n, loc=(0, -0.03, -0.06), rot=(R(90), 0, R(15)), parent=hand)
    slab(n + "_Ruecken", rrect(0.03, 0.17, 0.008, 4, 0.008, -0.04), 0.0, 0.004, parent=g, mat=toon(color))
    for i in range(12):
        y = -0.115 + i * 0.0135
        slab("%s_Zahn_%02d" % (n, i), rrect(0.03, 0.006, 0.0025, 3, -0.018, y), 0.0, 0.003, parent=g, mat=toon(color))
    log("Kamm")
    yield 0.2


def check_badge(n, parent, cx, cy, r, z, color=MINT):
    """Runder Haken-Button (flach) in einer senkrechten Gruppe."""
    slab(n + "_Kreis", ellipse(cx, cy, r, r, 48), z, z + 0.002, parent=parent, mat=flat(color), line=False)
    pts = [(cx - 0.45 * r, cy + 0.02 * r), (cx - 0.12 * r, cy - 0.3 * r), (cx + 0.48 * r, cy + 0.36 * r)]
    slab(n + "_Haken", band(pts, 0.2 * r, 0.2 * r, round_end=True, round_start=True), z + 0.0021, z + 0.0035,
         parent=parent, mat=flat(WHITE), line=False)


def calendar_board(n, root, band_color=CORAL, w=0.62, h=0.7):
    """Kalenderblatt auf einer Staffelei, ein Tag mit grossem Haken."""
    leg = toon(NIGHT)
    for sx in (-1, 1):
        capsule("%s_Bein_%d" % (n, sx), 1.25, 0.018, 0.018, parent=root, mat=leg,
                loc=(sx * 0.2, 0.1, 1.22), rot=(R(-12), R(-sx * 6), 0))
    capsule(n + "_Bein_hinten", 1.2, 0.016, 0.016, parent=root, mat=leg, loc=(0, 0.12, 1.2), rot=(R(22), 0, 0))
    yield
    g = empty(n + "_Blatt", loc=(0, -0.09, 0.5), rot=(R(78), 0, 0), parent=root)
    slab(n + "_Papier", rrect(w, h, 0.04, 8, 0, h / 2), 0.0, 0.012, parent=g, mat=toon(WHITE))
    slab(n + "_Kopf", rrect(w, 0.15, 0.04, 8, 0, h - 0.075), 0.012, 0.016, parent=g, mat=flat(band_color), line=False)
    slab(n + "_Kopf_unten", [(-w / 2, h - 0.15), (w / 2, h - 0.15), (w / 2, h - 0.11), (-w / 2, h - 0.11)], 0.012, 0.016,
         parent=g, mat=flat(band_color), line=False)
    for sx in (-1, 1):
        capsule("%s_Ring_%d" % (n, sx), 0.05, 0.012, 0.012, parent=g, mat=toon(NIGHT), loc=(sx * 0.17, h + 0.03, 0.02),
                rot=(R(90), 0, 0))
    log("Kalenderblatt")
    yield
    grey = flat(mix_hex(NIGHT, WHITE, 0.82))
    cols, rows = 5, 4
    cw, ch = (w - 0.12) / cols, (h - 0.27) / rows
    for r_ in range(rows):
        for c_ in range(cols):
            x = -w / 2 + 0.06 + cw * (c_ + 0.5)
            y = 0.06 + ch * (r_ + 0.5)
            slab("%s_Tag_%d_%d" % (n, r_, c_), rrect(cw * 0.62, ch * 0.62, 0.012, 4, x, y), 0.012, 0.014, parent=g,
                 mat=grey, line=False)
    log("Tage")
    yield
    x = -w / 2 + 0.06 + cw * 3.5
    y = 0.06 + ch * 2.5
    check_badge(n + "_Haken", g, x, y, 0.085, 0.015)
    log("Haken")
    yield


def hold_dryer(n, hand, target, body=YELLOW):
    """Foehn so in die Hand legen, dass die Duese auf target (Weltpunkt) zeigt, Griff in der Faust."""
    import bpy
    from mathutils import Vector, Matrix
    bpy.context.view_layer.update()
    hp = hand.matrix_world @ Vector((0, 0, -0.04))
    d = Vector(target) - hp
    d.z = 0.0
    d.normalize()
    side = Vector((0, 0, 1)).cross(d)
    rot = Matrix((d, side, Vector((0, 0, 1)))).transposed().to_euler()
    root = empty(n, rot=rot)
    root.location = hp - rot.to_matrix() @ Vector((0.085, 0, -0.1))
    yield from dryer(n + "_Geraet", root, body=body)


def envelope(n, parent, w=0.36, h=0.24, paper=WHITE, seal=CORAL, loc=(0, 0, 0), rot=(0, 0, 0)):
    """Briefumschlag (Vorderseite +z der Gruppe), Lasche mit Siegel."""
    g = empty(n, loc=loc, rot=rot, parent=parent)
    slab(n + "_Papier", rrect(w, h, 0.012, 4), 0.0, 0.012, parent=g, mat=toon(paper))
    fl = [(-w / 2 + 0.006, h / 2 - 0.006), (w / 2 - 0.006, h / 2 - 0.006), (0.0, -h * 0.12)]
    slab(n + "_Lasche", fl, 0.012, 0.016, parent=g, mat=toon(mix_hex(paper, SKY, 0.18)))
    for sx in (-1, 1):
        slab("%s_Falz_%d" % (n, sx), band([(sx * (w / 2 - 0.01), -h / 2 + 0.01), (sx * 0.05, -0.01)], 0.004, 0.004),
             0.012, 0.0135, parent=g, mat=flat(mix_hex(paper, NIGHT, 0.2)), line=False)
    slab(n + "_Siegel", ellipse(0, -h * 0.1, 0.034, 0.034, 40), 0.016, 0.02, parent=g, mat=toon(seal))
    return g


def bell(n, root, color=YELLOW, size=1.0):
    """Glocke, steht auf dem Boden."""
    s = size
    prof = [(0.36 * s, 0.0), (0.36 * s, 0.035 * s), (0.33 * s, 0.05 * s), (0.3 * s, 0.075 * s), (0.22 * s, 0.1 * s),
            (0.12 * s, 0.13 * s), (0.06 * s, 0.17 * s), (0.04 * s, 0.19 * s), (0.02 * s, 0.19 * s), (0.02 * s, 0.0)]
    lathe(n + "_Koerper", prof, n=56, parent=root, mat=toon(color), loc=(0, 0, 0.0))
    ellipsoid(n + "_Kloeppel", 0.05 * s, 0.05 * s, 0.05 * s, parent=root, mat=toon(mix_hex(color, CORAL, 0.6)),
              loc=(0.06 * s, -0.08 * s, 0.05 * s))
    ring_slab(n + "_Oese", ellipse(0, 0, 0.045 * s, 0.045 * s, 32), ellipse(0, 0, 0.025 * s, 0.025 * s, 32), -0.01 * s,
              0.01 * s, parent=root, mat=toon(color), loc=(0, 0, 0.385 * s), rot=(R(90), 0, 0))
    log("Glocke")
    yield


def big_phone(n, root, w=0.62, h=1.2, tilt=8, body=NIGHT, screen=WHITE):
    """Uebergrosses Handy, steht auf der Unterkante (leicht nach hinten gelehnt). Gibt die Display-Gruppe zurueck
    (lokal: x quer, y hoch ab Display-Unterkante, z nach vorn)."""
    g = empty(n, rot=(R(90 - tilt), 0, 0), parent=root)
    slab(n + "_Gehaeuse", rrect(w, h, 0.07, 10, 0, h / 2), -0.025, 0.025, parent=g, mat=toon(body, shade=0.3), bev=0.012, seg=3)
    d = empty(n + "_Display", loc=(0, 0.045, 0.026), parent=g)
    slab(n + "_Bildschirm", rrect(w - 0.06, h - 0.09, 0.05, 10, 0, (h - 0.09) / 2), 0.0, 0.002, parent=d,
         mat=flat(screen), line=False)
    slab(n + "_Lautsprecher", rrect(0.1, 0.012, 0.006, 4, 0, h - 0.03), 0.0, 0.002, parent=g, mat=flat(mix_hex(body, WHITE, 0.3)),
         line=False, loc=(0, 0, 0.026))
    log("grosses Handy")
    yield
    return d


def card_box(n, root, color=BLUE, w=0.56, d=0.34, h=0.22, tabs=(CORAL, MINT, YELLOW, SKY, CORAL, MINT)):
    """Offener Karteikasten mit stehenden Karten und farbigen Reitern."""
    bm = toon(color)
    ring_slab(n + "_Wand", rrect(w, d, 0.03, 6), rrect(w - 0.04, d - 0.04, 0.015, 6), 0.0, h, parent=root, mat=bm, bev=0.006, seg=2)
    slab(n + "_Boden", rrect(w - 0.02, d - 0.02, 0.02, 6), 0.0, 0.02, parent=root, mat=bm)
    log("Karteikasten")
    yield
    for i, tc in enumerate(tabs):
        y = -d / 2 + 0.05 + i * (d - 0.1) / (len(tabs) - 1)
        g = empty("%s_Karte_%d" % (n, i), loc=(0, y, 0.02), rot=(R(90 - 8), 0, 0), parent=root)
        hh = h + 0.04 + 0.012 * (i % 2)
        slab("%s_Karte_%d_Papier" % (n, i), rrect(w - 0.08, hh, 0.012, 4, 0, hh / 2), 0.0, 0.004, parent=g, mat=toon(WHITE))
        tx = -w / 2 + 0.1 + (i * 0.083) % (w - 0.2)
        slab("%s_Karte_%d_Reiter" % (n, i), rrect(0.09, 0.05, 0.012, 4, tx, hh + 0.012), 0.0, 0.004, parent=g, mat=toon(tc))
        if i % 2:
            yield 0.15
    log("Karten")
    yield


def customer_card(n, root, w=0.46, h=0.32, tilt=14):
    """Grosse Kundenkarte, lehnt (steht auf der Unterkante). Avatar, Name, Notiz, Terminhistorie."""
    g = empty(n, rot=(R(90 - tilt), 0, 0), parent=root)
    slab(n + "_Papier", rrect(w, h, 0.02, 6, 0, h / 2), 0.0, 0.008, parent=g, mat=toon(WHITE))
    z = 0.0085
    ink = flat(NIGHT)
    grey = flat(mix_hex(NIGHT, WHITE, 0.84))
    slab(n + "_Avatar", ellipse(-w / 2 + 0.07, h - 0.075, 0.042, 0.042, 40), z, z + 0.002, parent=g, mat=flat(K_SKY_ROSA), line=False)
    slab(n + "_Avatar_Kopf", ellipse(-w / 2 + 0.07, h - 0.068, 0.016, 0.018, 24), z + 0.0021, z + 0.003, parent=g,
         mat=flat(WHITE), line=False)
    slab(n + "_Name", rrect(0.17, 0.024, 0.012, 6, -w / 2 + 0.22, h - 0.06), z, z + 0.002, parent=g, mat=ink, line=False)
    slab(n + "_Tel", rrect(0.12, 0.016, 0.008, 6, -w / 2 + 0.195, h - 0.097), z, z + 0.002, parent=g, mat=grey, line=False)
    text(n + "_Notiz", "Farbe 7/1, 6 %", 0.026, font="Roboto-Medium.ttf", parent=g, loc=(-w / 2 + 0.03, h - 0.175, z), mat=ink)
    slab(n + "_Notiz_Linie", rrect(w - 0.06, 0.004, 0.002, 2, 0, h - 0.19), z, z + 0.002, parent=g, mat=grey, line=False)
    for i in range(3):
        y = h - 0.225 - i * 0.032
        check_badge("%s_Hist_%d" % (n, i), g, -w / 2 + 0.045, y, 0.011, z, color=MINT)
        slab("%s_Hist_Zeile_%d" % (n, i), rrect(0.22 - 0.04 * (i % 2), 0.014, 0.007, 4, -w / 2 + 0.18 - 0.02 * (i % 2), y),
             z, z + 0.002, parent=g, mat=grey, line=False)
    log("Kundenkarte")
    yield


K_SKY_ROSA = mix_hex(CORAL, WHITE, 0.55)


def tint_bowl(n, root, color=WHITE, paint=CORAL):
    """Farbschale mit Pinsel (Farbrezeptur)."""
    lathe(n + "_Schale", [(0.075, 0.0), (0.075, 0.1), (0.07, 0.105), (0.02, 0.08), (0.0, 0.05), (0.0, 0.0)], n=48,
          parent=root, mat=toon(color))
    slab(n + "_Farbe", ellipse(0, 0, 0.088, 0.088, 48), 0.0755, 0.077, parent=root, mat=flat(paint), line=False)
    capsule(n + "_Pinselstiel", 0.24, 0.011, 0.009, parent=root, mat=toon(NIGHT), loc=(-0.03, 0.0, 0.07), rot=(0, R(-72), R(-25)))
    log("Farbschale")
    yield


def pencil(n, root, color=YELLOW, L=0.2):
    """Bleistift, liegt auf dem Boden entlang +X."""
    g = empty(n, rot=(0, R(90), 0), parent=root)
    lathe(n + "_Holz", [(L, 0.0), (L - 0.035, 0.009), (0.0, 0.009), (0.0, 0.0)], n=6, parent=g, mat=toon(color),
          loc=(0, 0, 0))
    lathe(n + "_Spitze", [(L + 0.0, 0.0), (L - 0.012, 0.0035), (L - 0.013, 0.0)], n=12, parent=g, mat=toon(NIGHT))
    g.location.z = 0.009
    log("Stift")
    yield


def desk(n, root, top=WHITE, legs=NIGHT, w=1.3, d=0.62, h=0.74):
    loft_z(n + "_Platte", [(h - 0.04, w / 2, d / 2, 8), (h, w / 2, d / 2, 8)], n=64, parent=root, mat=toon(top))
    for sx in (-1, 1):
        for sy in (-1, 1):
            capsule("%s_Bein_%d_%d" % (n, sx, sy), h - 0.06, 0.022, 0.02, parent=root, mat=toon(legs),
                    loc=(sx * (w / 2 - 0.08), sy * (d / 2 - 0.08), h - 0.04))
    log("Tisch")
    yield


def monitor(n, root, w=0.86, h=0.52, body=NIGHT):
    """Bildschirm auf Fuss (steht auf einer Flaeche). Gibt Display-Gruppe zurueck (x quer, y hoch, z vorn)."""
    bm = toon(body, shade=0.3)
    loft_z(n + "_Fuss", [(0.0, 0.13, 0.09, 4), (0.018, 0.13, 0.09, 4), (0.02, 0.0, 0.0, 4)], parent=root, mat=bm)
    capsule(n + "_Hals", 0.2, 0.022, 0.022, parent=root, mat=bm, loc=(0, 0.03, 0.24))
    g = empty(n + "_Rahmen_Gruppe", loc=(0, 0.0, 0.2), rot=(R(86), 0, 0), parent=root)
    slab(n + "_Rahmen", rrect(w, h, 0.03, 8, 0, h / 2), -0.015, 0.015, parent=g, mat=bm, bev=0.006, seg=2)
    d = empty(n + "_Display", loc=(0, 0.025, 0.016), parent=g)
    slab(n + "_Bildschirm", rrect(w - 0.04, h - 0.05, 0.015, 8, 0, (h - 0.05) / 2), 0.0, 0.002, parent=d,
         mat=flat(WHITE), line=False)
    log("Bildschirm")
    yield
    return d


def week_grid(n, d, w, h, staff=(CORAL, MINT, YELLOW, SKY)):
    """Wochenkalender im Display: 5 Tage, Termine als farbige Bloecke je Mitarbeiter, ein Termin mit Haken."""
    grey = flat(mix_hex(NIGHT, WHITE, 0.88))
    z = 0.0022
    slab(n + "_Kopf", rrect(w, 0.05, 0.012, 4, 0, h - 0.025), z, z + 0.001, parent=d, mat=flat(SKY_L), line=False)
    days = ["Mo", "Di", "Mi", "Do", "Fr"]
    cw = (w - 0.06) / 5
    for i, t in enumerate(days):
        x = -w / 2 + 0.06 + cw * (i + 0.5)
        text("%s_Tag_%d" % (n, i), t, 0.026, font="Roboto-Bold.ttf", align="CENTER", parent=d,
             loc=(x, h - 0.036, z + 0.0012), mat=flat(NIGHT))
        slab("%s_Spalte_%d" % (n, i), [(x - cw / 2, 0.01), (x - cw / 2 + 0.003, 0.01), (x - cw / 2 + 0.003, h - 0.055),
                                       (x - cw / 2, h - 0.055)], z, z + 0.001, parent=d, mat=grey, line=False)
    for k in range(5):
        y = h - 0.09 - k * (h - 0.11) / 5
        slab("%s_Zeile_%d" % (n, k), [(-w / 2 + 0.06, y), (w / 2, y), (w / 2, y + 0.002), (-w / 2 + 0.06, y + 0.002)],
             z, z + 0.001, parent=d, mat=grey, line=False)
        text("%s_Uhr_%d" % (n, k), "%d" % (9 + 2 * k), 0.018, font="Roboto-Medium.ttf", align="CENTER", parent=d,
             loc=(-w / 2 + 0.03, y - 0.01, z + 0.0012), mat=flat(mix_hex(NIGHT, WHITE, 0.4)))
    # (Tag, Start 0..1, Dauer, Mitarbeiter)
    appts = [(0, 0.05, 0.18, 0), (0, 0.42, 0.14, 1), (1, 0.12, 0.22, 2), (1, 0.55, 0.16, 0), (2, 0.02, 0.12, 3),
             (2, 0.3, 0.2, 1), (2, 0.66, 0.14, 2), (3, 0.18, 0.26, 0), (3, 0.6, 0.12, 3), (4, 0.08, 0.16, 1),
             (4, 0.36, 0.18, 2), (4, 0.7, 0.14, 0)]
    top, span = h - 0.07, h - 0.1
    for i, (day, s0, dur, st) in enumerate(appts):
        x = -w / 2 + 0.06 + cw * (day + 0.5)
        y1 = top - s0 * span
        y0 = y1 - dur * span
        slab("%s_Termin_%d" % (n, i), rrect(cw - 0.016, y1 - y0, 0.01, 4, x, (y0 + y1) / 2), z, z + 0.0015, parent=d,
             mat=flat(staff[st]), line=False)
        slab("%s_Termin_%d_Rand" % (n, i), rrect(0.006, y1 - y0 - 0.01, 0.003, 2, x - cw / 2 + 0.014, (y0 + y1) / 2),
             z + 0.0016, z + 0.002, parent=d, mat=flat(mix_hex(staff[st], NIGHT, 0.35)), line=False)
    log("Wochenraster")
    yield


def wall_clock(n, root, r=0.2, hh=10, mi=30, rim=NIGHT, face=WHITE, accent=CORAL):
    """Wanduhr fuer eine Wand (root = senkrechte Gruppe, Vorderseite +z)."""
    ring_slab(n + "_Rand", ellipse(0, 0, r, r, 64), ellipse(0, 0, r * 0.88, r * 0.88, 64), 0.0, 0.05, parent=root,
              mat=toon(rim, shade=0.3))
    slab(n + "_Blatt", ellipse(0, 0, r * 0.89, r * 0.89, 64), 0.0, 0.03, parent=root, mat=toon(face))
    for i in range(12):
        a = R(i * 30)
        L = 0.05 if i % 3 == 0 else 0.03
        c = [(math.sin(a) * (r * 0.8 - L * r), math.cos(a) * (r * 0.8 - L * r)), (math.sin(a) * r * 0.8, math.cos(a) * r * 0.8)]
        slab("%s_Strich_%d" % (n, i), band(c, 0.012 * r / 0.2, 0.012 * r / 0.2, round_end=False), 0.031, 0.033,
             parent=root, mat=flat(NIGHT), line=False)
    for nm, ang, L, wd in (("Std", (hh % 12 + mi / 60) * 30, 0.5, 0.035), ("Min", mi * 6, 0.72, 0.026)):
        a = R(ang)
        c = [(-math.sin(a) * 0.08 * r, -math.cos(a) * 0.08 * r), (math.sin(a) * L * r, math.cos(a) * L * r)]
        slab("%s_%s" % (n, nm), band(c, wd * r / 0.2, wd * r / 0.2 * 0.7, round_start=True), 0.034, 0.038, parent=root,
             mat=flat(NIGHT), line=False)
    slab(n + "_Mitte", ellipse(0, 0, 0.018 * r / 0.2, 0.018 * r / 0.2, 24), 0.038, 0.041, parent=root, mat=flat(accent), line=False)
    log("Wanduhr")
    yield


def wall_shelf(n, root, w=0.66, bottles=(CORAL, MINT, YELLOW, SKY)):
    """Wandregal mit Pflegeflaschen (root = Weltposition der Regal-Mitte an der Wand, Regal ragt nach -Y)."""
    slab(n + "_Brett", rrect(w, 0.2, 0.015, 4, 0, -0.1), 0.0, 0.03, parent=root, mat=toon(WHITE))
    for sx in (-1, 1):
        slab("%s_Halter_%d" % (n, sx), [(0.0, 0.0), (-0.14, 0.0), (0.0, -0.12)], -0.012, 0.012, parent=root,
             mat=toon(NIGHT), loc=(sx * (w / 2 - 0.08), -0.005, 0.0), rot=(R(90), 0, R(90)))
    yield
    xs = [-w / 2 + 0.1 + i * (w - 0.2) / max(1, len(bottles) - 1) for i in range(len(bottles))]
    for i, (x, c) in enumerate(zip(xs, bottles)):
        tall = 0.22 if i % 2 == 0 else 0.16
        prof = [(tall + 0.05, 0.0), (tall + 0.05, 0.018), (tall + 0.02, 0.018), (tall + 0.015, 0.014), (tall, 0.04),
                (tall - 0.02, 0.048), (0.0, 0.048), (0.0, 0.0)]
        lathe("%s_Flasche_%d" % (n, i), prof, n=36, parent=root, mat=toon(c), loc=(x, -0.1, 0.03))
        slab("%s_Etikett_%d" % (n, i), rrect(0.05, tall * 0.4, 0.008, 4, 0, 0), 0.0, 0.002, parent=root,
             mat=flat(WHITE), line=False, loc=(x, -0.1 - 0.049, 0.03 + tall * 0.45), rot=(R(90), 0, 0))
    log("Regal mit Flaschen")
    yield
