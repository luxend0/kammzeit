# Stilisierte Menschen aus einfachen Formen (Kopf-Ellipsoid, Kapsel-Glieder, Loft-Rumpf).
# Figur schaut nach -Y (zur Kamera). Rechte Koerperseite liegt bei -X.
# person(...) ist ein Generator (jedes yield = ein sichtbarer Bauschritt) und gibt am Ende
# ein dict mit Gelenken zurueck (hand_r, hand_l, head, pelvis), z. B. um Dinge in die Hand zu legen.
import math
from kzlib import (toon, flat, empty, capsule, ellipsoid, loft_z, slab, band, log, NIGHT, WHITE)

R = math.radians


def _rot(t):
    return tuple(R(a) for a in t)


def dome(name, rx, ry, rz, theta_max, tilt, parent, mat, loc=(0, 0, 0), k=14):
    st = []
    for i in range(k + 1):
        th = R(theta_max) * i / k
        s = math.sin(th)
        st.append((rz * math.cos(th), max(rx * s, 0.0), max(ry * s, 0.0), 2.0))
    return loft_z(name, st, n=48, parent=parent, mat=mat, caps=False, loc=loc, rot=(R(tilt), 0, 0))


def hair(n, style, headc, mat):
    """Frisuren relativ zum Kopfmittelpunkt (Kopf ~ 0.115 x 0.11 x 0.13)."""
    if style == "none":
        return
    if style == "afro":
        ellipsoid(n + "_Haar", 0.165, 0.15, 0.155, parent=headc, mat=mat, loc=(0, 0.055, 0.055))
        return
    if style == "beard":
        dome(n + "_Haar", 0.122, 0.118, 0.137, 92, -26, headc, mat)
        ellipsoid(n + "_Bart", 0.1, 0.075, 0.05, parent=headc, mat=mat, loc=(0, -0.05, -0.092))
        return
    tilt = {"short": -24, "bun": -30, "long": -30, "ponytail": -28, "quiff": -20, "bob": -30}.get(style, -26)
    th = {"short": 100, "quiff": 98, "bob": 108}.get(style, 104)
    dome(n + "_Haar", 0.124, 0.12, 0.142 if style != "quiff" else 0.135, th, tilt, headc, mat)
    if style == "bun":
        ellipsoid(n + "_Dutt", 0.062, 0.06, 0.058, parent=headc, mat=mat, loc=(0, 0.075, 0.125))
    elif style == "long":
        ellipsoid(n + "_Haar_hinten", 0.132, 0.07, 0.21, parent=headc, mat=mat, loc=(0, 0.065, -0.11))
    elif style == "bob":
        ellipsoid(n + "_Haar_Bob", 0.138, 0.11, 0.115, parent=headc, mat=mat, loc=(0, 0.035, -0.03))
    elif style == "ponytail":
        ellipsoid(n + "_Zopf", 0.045, 0.045, 0.14, parent=headc, mat=mat, loc=(0, 0.16, -0.03), rot=(R(28), 0, 0))
    elif style == "quiff":
        ellipsoid(n + "_Tolle", 0.1, 0.075, 0.055, parent=headc, mat=mat, loc=(0, -0.035, 0.122), rot=(R(-18), 0, 0))


def face(n, headc, skin_hex):
    eye = flat(NIGHT)
    for sx in (-1, 1):
        ellipsoid(n + "_Auge_%s" % ("R" if sx < 0 else "L"), 0.011, 0.006, 0.015, parent=headc, mat=eye,
                  loc=(sx * 0.042, -0.104, 0.004), line=False)
    arc = [(0.022 * math.cos(R(a)), 0.022 * math.sin(R(a)) + 0.012) for a in range(205, 336, 10)]
    slab(n + "_Mund", band(arc, 0.006, 0.006, round_end=True, round_start=True), 0.0, 0.002, parent=headc,
         mat=eye, line=False, loc=(0, -0.106, -0.05), rot=(R(90), 0, 0))


def person(n, root, look, pose):
    """look: skin, hair, hair_color, top, bottom, shoes, sleeves ('short'|'long'), dress, cape, apron
    pose: leg_r/leg_l = ((hip x, y, z), knee), arm_r/arm_l = ((shoulder x, y, z), elbow),
          torso = (x, y, z), head = (x, y, z), pelvis = (x, y, z)."""
    L, P = look, pose
    skin = toon(L["skin"])
    top = toon(L["top"])
    bottom = toon(L["bottom"])
    shoe = toon(L.get("shoes", WHITE))
    hairm = toon(L.get("hair_color", NIGHT))
    pelvis = empty(n + "_Becken", parent=root, rot=_rot(P.get("pelvis", (0, 0, 0))))
    joints = {"pelvis": pelvis}
    # Beine
    for side, sx in (("r", -1), ("l", 1)):
        (hx, hy, hz), knee_a = P.get("leg_" + side, ((0, 0, 0), 0))
        hip = empty("%s_Huefte_%s" % (n, side), loc=(sx * 0.088, 0, 0), rot=_rot((hx, hy, hz)), parent=pelvis)
        legm = skin if L.get("dress") and L.get("bare_legs", True) else bottom
        capsule("%s_Oberschenkel_%s" % (n, side), 0.39, 0.078, 0.06, parent=hip, mat=bottom if not L.get("dress") else legm)
        knee = empty("%s_Knie_%s" % (n, side), loc=(0, 0, -0.39), rot=(R(knee_a), 0, 0), parent=hip)
        capsule("%s_Unterschenkel_%s" % (n, side), 0.37, 0.058, 0.046, parent=knee, mat=legm)
        ankle = empty("%s_Knoechel_%s" % (n, side), loc=(0, 0, -0.37), rot=(R(-(hx + knee_a)), 0, 0), parent=knee)
        ellipsoid("%s_Schuh_%s" % (n, side), 0.058, 0.118, 0.048, parent=ankle, loc=(0, -0.05, -0.04), mat=shoe)
        log("Bein", n, side)
        yield 0.2
    torso = empty(n + "_Rumpf", parent=pelvis, rot=_rot(P.get("torso", (0, 0, 0))))
    loft_z(n + "_Hose", [(-0.085, 0.0, 0.0, 2), (-0.08, 0.1, 0.07, 2.4), (-0.05, 0.158, 0.102, 2.6),
                         (0.04, 0.162, 0.104, 2.6), (0.12, 0.15, 0.1, 2.6)], parent=torso, mat=bottom)
    loft_z(n + "_Oberteil", [(0.08, 0.158, 0.106, 2.6), (0.22, 0.146, 0.1, 2.6), (0.38, 0.168, 0.106, 2.6),
                             (0.48, 0.182, 0.1, 2.8), (0.54, 0.16, 0.09, 2.6), (0.58, 0.085, 0.06, 2.2),
                             (0.595, 0.0, 0.0, 2)], parent=torso, mat=top)
    if L.get("dress"):
        loft_z(n + "_Rock", [(0.2, 0.15, 0.1, 2.6), (0.05, 0.19, 0.14, 2.4), (-0.25, 0.25, 0.2, 2.2),
                             (-0.36, 0.265, 0.215, 2.2)], parent=torso, mat=top, caps=False)
    if L.get("apron"):
        ap = toon(L["apron"])
        lo = L.get("apron_low", -0.24)
        loft_z(n + "_Schuerze", [(0.42, 0.13, 0.112, 2.4, 0, -0.004), (0.2, 0.152, 0.11, 2.4, 0, -0.006),
                                 (0.0, 0.17, 0.116, 2.4, 0, -0.008), (lo, 0.185, 0.126, 2.4, 0, -0.012)],
               parent=torso, mat=ap, caps=False)
    log("Rumpf", n)
    yield
    # Arme
    long_sleeve = L.get("sleeves", "short") == "long"
    for side, sx in (("r", -1), ("l", 1)):
        if L.get("cape"):
            break  # Arme liegen unter dem Umhang
        (ax, ay, az), elbow_a = P.get("arm_" + side, ((0, sx * -8, 0), -10))
        sh = empty("%s_Schulter_%s" % (n, side), loc=(sx * 0.178, 0, 0.47), rot=_rot((ax, ay, az)), parent=torso)
        capsule("%s_Oberarm_%s" % (n, side), 0.27, 0.05, 0.043, parent=sh, mat=top)
        el = empty("%s_Ellbogen_%s" % (n, side), loc=(0, 0, -0.27), rot=(R(elbow_a), 0, 0), parent=sh)
        capsule("%s_Unterarm_%s" % (n, side), 0.235, 0.042, 0.034, parent=el, mat=top if long_sleeve else skin)
        hand = empty("%s_Hand_%s" % (n, side), loc=(0, 0, -0.235), parent=el)
        ellipsoid("%s_Handflaeche_%s" % (n, side), 0.036, 0.026, 0.05, parent=hand, loc=(0, 0, -0.035), mat=skin)
        joints["hand_" + side] = hand
        log("Arm", n, side)
        yield 0.2
    if L.get("cape"):
        cp = toon(L["cape"])
        loft_z(n + "_Umhang", [(0.62, 0.05, 0.045, 2), (0.57, 0.18, 0.13, 2.2), (0.48, 0.275, 0.18, 2.4),
                               (0.28, 0.3, 0.21, 2.5, 0, -0.02), (0.1, 0.31, 0.23, 2.5, 0, -0.05),
                               (0.0, 0.315, 0.245, 2.6, 0, -0.07)], parent=torso, mat=cp, caps=False)
        log("Umhang", n)
        yield
    capsule(n + "_Hals", 0.06, 0.043, 0.043, parent=torso, mat=skin, loc=(0, 0, 0.635))
    head = empty(n + "_Kopf", loc=(0, 0, 0.62), rot=_rot(P.get("head", (0, 0, 0))), parent=torso)
    headc = empty(n + "_Kopfmitte", loc=(0, 0, 0.135), parent=head)
    ellipsoid(n + "_Kopfform", 0.115, 0.11, 0.13, parent=headc, mat=skin)
    for sx in (-1, 1):
        ellipsoid(n + "_Ohr_%s" % ("R" if sx < 0 else "L"), 0.018, 0.025, 0.032, parent=headc, mat=skin,
                  loc=(sx * 0.112, 0.005, -0.005))
    joints["head"] = headc
    log("Kopf", n)
    yield
    face(n, headc, L["skin"])
    hair(n, L.get("hair", "short"), headc, hairm)
    log("Gesicht und Haare", n)
    yield
    return joints


# Fertige Posen
STAND = {}
SIT = {"leg_r": ((-90, 0, -4), 90), "leg_l": ((-90, 0, 4), 90),
       "arm_r": ((-20, 6, 0), -60), "arm_l": ((-20, -6, 0), -60)}
