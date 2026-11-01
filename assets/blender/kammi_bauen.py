# Kammi, der Kammzeit-Igel: baut das Maskottchen sichtbar auf, speichert kammi.blend
# (Collection "Kammi") und rendert eine Vorschau.
#
# Aufbau (Namen sind Schnittstelle fuer toon_lib.pose_kammi / append_kammi):
#   Kammi                 Wurzel-Empty auf dem Boden, Blick nach -Y, ~0.62 m hoch
#   +- Kammi_Rumpf        Empty (Huefte), Neigen/Kippen des Oberkoerpers
#   |  +- Koerper, Fell, Stacheln, Tolle, Kamm, Gesicht (Schnauze, Nase, Augen, Brauen, Mund, Wangen, Ohren)
#   |  +- Kammi_Schulter_L/R   Empty, Arm zeigt in Ruhe nach -Z; Kammi_Hand_L/R = Empty fuer Requisiten
#   +- Kammi_Fuss_L/R     Empty mit Fuss
# L = Kammis linke Seite = +X (Betrachter rechts).
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bpy, bmesh
from mathutils import Vector
import toon_lib as T

OUT_BLEND = T.KAMMI_BLEND
OUT_PNG = os.path.join(T.ASSETS, "ill", "test", "kammi-vorschau.png")

# Farben (Palette)
C_FELL = "blau"          # Stacheln und Fell
C_GESICHT = "creme"      # Gesicht, Bauch, Arme, Fuesse
C_BAUCH = T.mix("creme", "weiss", 0.65)
C_DUNKEL = "nachtblau"   # Augen, Nase, Mund
C_WANGE = "rosa"
C_KAMM = "gelb"

PIVOT = Vector((0, 0, 0.10))
BODY_C = Vector((0, 0.02, 0.29))
BODY_R = Vector((0.19, 0.175, 0.245))


def L(v):
    """Weltkoordinate (Kammi steht im Ursprung) -> lokal zu Kammi_Rumpf."""
    return Vector(v) - PIVOT


def on_body(az, el, grow=0.0):
    """Punkt + Normale auf dem Koerper-Ellipsoid. az=0 hinten (+Y), az=180 vorne (-Y), el = Hoehe."""
    a, e = math.radians(az), math.radians(el)
    d = Vector((math.sin(a) * math.cos(e), math.cos(a) * math.cos(e), math.sin(e)))
    R = BODY_R + Vector((grow, grow, grow))
    p = BODY_C + Vector((R.x * d.x, R.y * d.y, R.z * d.z))
    n = Vector((d.x / R.x, d.y / R.y, d.z / R.z)).normalized()
    t = Vector((R.x * math.cos(a) * math.cos(e), -R.y * math.sin(a) * math.cos(e), 0))
    if t.length < 1e-6:
        t = Vector((1, 0, 0))
    return p, n, t.normalized()


FACE_DIR = Vector((0, -1, -0.09)).normalized()


def mask_radius(psi):
    """Radius (Grad) der Gesichtsmaske um FACE_DIR; psi=0 oben (Stirnspitze), pi unten."""
    return 52 - 16 * max(0.0, math.cos(psi)) ** 14 + 12 * max(0.0, -math.cos(psi)) ** 2


def in_face(dn, margin=0.0):
    """Liegt die (normierte) Richtung dn in der Gesichts-/Bauchoeffnung des Fells?
    Runde Maske mit kleiner Spitze in der Stirnmitte, Bauch unten offen."""
    ang = math.degrees(math.acos(max(-1, min(1, dn.dot(FACE_DIR)))))
    up = Vector((0, 0, 1)) - FACE_DIR * FACE_DIR.z
    up.normalize()
    side = FACE_DIR.cross(up)
    psi = math.atan2(dn.dot(side), dn.dot(up))
    return ang < mask_radius(psi) + margin


def tooth(bm, p0, p1, p2, side, w0, w1, th, n_st=9, seg=16):
    """Flacher Kammzahn entlang einer quadratischen Bezierkurve, runde Spitze."""
    rings = []
    for i in range(n_st + 1):
        t = i / n_st
        P = p0 * (1 - t) ** 2 + p1 * (2 * (1 - t) * t) + p2 * (t * t)
        Tg = (p1 - p0) * (2 * (1 - t)) + (p2 - p1) * (2 * t)
        Tg.normalize()
        S = (side - Tg * side.dot(Tg)).normalized()
        N = Tg.cross(S)
        w = w0 + (w1 - w0) * t
        k = 1.0
        if t > 0.72:
            k = max(0.10, math.sqrt(max(0.0, 1 - ((t - 0.72) / 0.28) ** 2)))
        ring = []
        for j in range(seg):
            a = 2 * math.pi * j / seg
            c, s = math.cos(a), math.sin(a)
            x = math.copysign(abs(c) ** 0.6, c)
            y = math.copysign(abs(s) ** 0.6, s)
            ring.append(bm.verts.new(P + S * (w / 2 * k * x) + N * (th / 2 * max(k, 0.45) * y)))
        rings.append(ring)
    for a, b in zip(rings, rings[1:]):
        for j in range(seg):
            q = (j + 1) % seg
            bm.faces.new((a[j], a[q], b[q], b[j]))
    bm.faces.new(list(reversed(rings[0])))
    bm.faces.new(rings[-1])


def build():
    T.set_light(-130, 48)
    root = T.empty("Kammi", size=0.1)
    rumpf = T.empty("Kammi_Rumpf", loc=PIVOT, parent=root, size=0.08)
    T.header("Kammi: Koerper")
    # Koerper (Bauch/Gesicht creme), leicht birnenfoermig
    body = T.ellipsoid("Kammi_Koerper", L(BODY_C), BODY_R, color=C_GESICHT, steps=3, parent=rumpf, seg=48, rings=32)
    me = body.data
    for v in me.vertices:
        zn = v.co.z / BODY_R.z
        f = 1.0 + 0.10 * max(0.0, -zn)            # unten etwas breiter
        v.co.x *= f
        v.co.y *= f
    yield
    T.ellipsoid("Kammi_Bauch", L((0, -0.158, 0.145)), (0.072, 0.03, 0.075), color=C_BAUCH, steps=2, parent=rumpf)
    yield

    # Fell: geschlossenes Ellipsoid (blau), darueber die Gesichtsmaske als glatte Flaeche
    T.header("Kammi: Fell und Gesichtsmaske")
    T.ellipsoid("Kammi_Fell", L(BODY_C), BODY_R + Vector((0.01, 0.01, 0.01)), color=C_FELL, steps=2, parent=rumpf,
                seg=48, rings=32)
    yield
    bm = bmesh.new()
    up = Vector((0, 0, 1)) - FACE_DIR * FACE_DIR.z
    up.normalize()
    side = FACE_DIR.cross(up)
    NR, NS = 22, 96
    grid = []
    for i in range(NR + 1):
        ring = []
        for j in range(NS):
            psi = 2 * math.pi * j / NS
            rad = mask_radius(psi) * i / NR
            r = math.radians(rad)
            dn = (FACE_DIR * math.cos(r) + (up * math.cos(psi) + side * math.sin(psi)) * math.sin(r)).normalized()
            G = BODY_R + Vector((0.018, 0.018, 0.018))
            p = BODY_C + Vector((G.x * dn.x, G.y * dn.y, G.z * dn.z))
            ring.append(bm.verts.new(L(p)))
            if i == 0:
                break
        grid.append(ring)
    c0 = grid[0][0]
    for j in range(NS):
        q = (j + 1) % NS
        bm.faces.new((c0, grid[1][q], grid[1][j]))
    for i in range(1, NR):
        a_, b_ = grid[i], grid[i + 1]
        for j in range(NS):
            q = (j + 1) % NS
            bm.faces.new((a_[j], b_[j], b_[q], a_[q]))
    bmesh.ops.recalc_face_normals(bm, faces=bm.faces[:])
    # Normalen muessen nach aussen (weg vom Koerper) zeigen
    f0 = bm.faces[0]
    if f0.normal.dot(f0.calc_center_median() - L(BODY_C)) < 0:
        bmesh.ops.reverse_faces(bm, faces=bm.faces[:])
    T.mesh_obj("Kammi_Gesicht", bm, C_GESICHT, 3, rumpf)
    yield

    # Stacheln: Reihen von Kammzaehnen, nach hinten gestrichen
    T.header("Kammi: Stacheln wie Kammzaehne")
    rows = [(-28, 100, 9, 0.10), (-8, 118, 11, 0.12), (12, 132, 12, 0.13), (32, 150, 13, 0.135),
            (50, 170, 13, 0.13), (66, 180, 11, 0.12), (80, 180, 6, 0.10)]
    back = Vector((0, 1, 0.3)).normalized()
    for ri, (el, azmax, n, length) in enumerate(rows):
        bm = bmesh.new()
        cnt = 0
        for k in range(n):
            off = 0.5 if ri % 2 else 0.0
            u = (k + off) / (n - 1 + off) if n > 1 else 0.5
            az = -azmax + 2 * azmax * u
            p, nn, tg = on_body(az, el, grow=-0.01)
            dn = Vector(((p - BODY_C).x / BODY_R.x, (p - BODY_C).y / BODY_R.y, (p - BODY_C).z / BODY_R.z)).normalized()
            if in_face(dn, margin=4):
                continue
            d = (nn * 0.8 + back * 0.6).normalized()
            p0 = L(p)
            p1 = p0 + d * length * 0.55 + nn * length * 0.10
            p2 = p0 + (d * 0.85 + back * 0.15).normalized() * length
            tooth(bm, p0, p1, p2, tg, 0.046, 0.032, 0.019)
            cnt += 1
        if cnt:
            T.mesh_obj("Kammi_Stacheln_%d" % (ri + 1), bm, C_FELL, 2, rumpf)
        else:
            bm.free()
        yield 0.3

    # Tolle: fuenf groessere Zaehne ueber der Stirn, nach oben und vorne geschwungen
    T.header("Kammi: Frisur (Tolle)")
    bm = bmesh.new()
    for k, (az, el, h, spread) in enumerate(((158, 50, 0.16, -1.0), (171, 54, 0.20, -0.35), (184, 54, 0.21, 0.35), (197, 50, 0.17, 1.0))):
        p, nn, tg = on_body(az, el, grow=-0.014)
        p0 = L(p)
        sc = h / 0.2
        p1 = p0 + Vector((spread * 0.02, -0.08, 0.10)) * sc
        p2 = p0 + Vector((spread * 0.05, 0.03, 0.17)) * sc
        side = Vector((1, 0, -spread * 0.35)).normalized()
        tooth(bm, p0, p1, p2, side, 0.058, 0.040, 0.022, n_st=12)
    T.mesh_obj("Kammi_Tolle", bm, C_FELL, 2, rumpf)
    yield
    # kleiner gelber Kamm in der Tolle (Markenzeichen)
    kamm = T.slab("Kammi_Kamm", [(-0.05, -0.012), (0.05, -0.012), (0.05, 0.012), (-0.05, 0.012)], -0.004, 0.004,
                  color=C_KAMM, steps=2, parent=rumpf, radius=0.003)
    for i in range(8):
        x = -0.042 + i * 0.012
        T.slab("Kammi_Kamm_Zahn_%d" % i, [(x - 0.0035, -0.04), (x + 0.0035, -0.04), (x + 0.0035, -0.008), (x - 0.0035, -0.008)],
               -0.0035, 0.0035, color=C_KAMM, steps=2, parent=kamm, radius=0.0015)
    kamm.location = L((0.075, -0.02, 0.595))
    kamm.rotation_euler = (math.radians(90), math.radians(-35), math.radians(-25))
    yield

    # Gesicht
    T.header("Kammi: Gesicht")
    T.ellipsoid("Kammi_Schnauze", L((0, -0.195, 0.25)), (0.074, 0.105, 0.062), color=C_GESICHT, steps=3,
                parent=rumpf)
    yield
    T.ellipsoid("Kammi_Nase", L((0, -0.298, 0.266)), (0.03, 0.022, 0.024), color=C_DUNKEL, steps=3, parent=rumpf)
    yield
    for side, sx in (("L", 1), ("R", -1)):
        T.ellipsoid("Kammi_Auge_%s" % side, L((sx * 0.072, -0.176, 0.345)), (0.024, 0.014, 0.034), color=C_DUNKEL, steps=2,
                    rot=(0, 0, sx * -18), parent=rumpf)
        T.ellipsoid("Kammi_Glanz_%s" % side, L((sx * 0.072 + 0.009, -0.189, 0.357)), (0.008, 0.005, 0.009),
                    color=T.flat("weiss"), parent=rumpf)["kz_no_outline"] = 1
        # Brauen: leicht schraeg, freundlich-selbstbewusst
        T.tube("Kammi_Braue_%s" % side, [L((sx * 0.047, -0.176, 0.41)), L((sx * 0.072, -0.171, 0.416)), L((sx * 0.096, -0.16, 0.404))],
               0.0052, color=T.flat(C_DUNKEL), parent=rumpf, seg=10)
        # geschlossene Augen (fuer "schlaeft"), versteckt
        zu = T.tube("Kammi_Augezu_%s" % side, [L((sx * 0.05, -0.186, 0.345)), L((sx * 0.072, -0.191, 0.336)), L((sx * 0.094, -0.182, 0.345))],
                    0.0055, color=T.flat(C_DUNKEL), parent=rumpf, seg=10)
        zu.hide_render = True
        zu.hide_viewport = True
        T.ellipsoid("Kammi_Wange_%s" % side, L((sx * 0.112, -0.157, 0.272)), (0.028, 0.012, 0.019), color=T.flat(C_WANGE),
                    rot=(0, 0, sx * -38), parent=rumpf)["kz_no_outline"] = 1
        # Ohren
        ear = T.ellipsoid("Kammi_Ohr_%s" % side, L((sx * 0.125, -0.035, 0.455)), (0.034, 0.018, 0.04), color=C_GESICHT, steps=2,
                          rot=(0, sx * 25, sx * -20), parent=rumpf)
        yield 0.3
    def on_snout(x, z):
        c, r = Vector((0, -0.195, 0.25)), Vector((0.074, 0.105, 0.062))
        q = max(0.0, 1 - (x / r.x) ** 2 - ((z - c.z) / r.z) ** 2)
        return L((x, c.y - r.y * math.sqrt(q) - 0.0035, z))
    T.tube("Kammi_Mund", [on_snout(x, z) for x, z in ((-0.043, 0.237), (-0.024, 0.222), (0.0, 0.216), (0.024, 0.222), (0.043, 0.237))],
           0.0052, color=T.flat(C_DUNKEL), parent=rumpf, seg=10)
    yield

    # Arme
    T.header("Kammi: Arme")
    for side, sx in (("L", 1), ("R", -1)):
        sh = T.empty("Kammi_Schulter_%s" % side, loc=L((sx * 0.162, -0.07, 0.255)), parent=rumpf, size=0.04)
        sh.rotation_mode = "QUATERNION"
        T.capsule("Kammi_Arm_%s" % side, (0, 0, 0), (0, 0, -0.105), 0.031, 0.028, color=C_GESICHT, steps=3, parent=sh)
        T.ellipsoid("Kammi_Pfote_%s" % side, (0, 0, -0.125), (0.038, 0.034, 0.04), color=C_GESICHT, steps=3, parent=sh)
        T.empty("Kammi_Hand_%s" % side, loc=(0, 0, -0.13), parent=sh, size=0.03)
        yield 0.3
    # Fuesse
    T.header("Kammi: Fuesse")
    for side, sx in (("L", 1), ("R", -1)):
        fe = T.empty("Kammi_Fuss_%s" % side, loc=(sx * 0.082, -0.03, 0.0), parent=root, size=0.04)
        T.capsule("Kammi_Bein_%s" % side, (0, 0.01, 0.12), (0, -0.01, 0.04), 0.04, 0.036, color=C_GESICHT, steps=3, parent=fe)
        T.ellipsoid("Kammi_Fussform_%s" % side, (0, -0.04, 0.032), (0.05, 0.078, 0.034), color=C_GESICHT, steps=3, parent=fe)
        yield 0.3

    # in Sammlung "Kammi" verschieben
    col = bpy.data.collections.new("Kammi")
    T.SCENE.collection.children.link(col)
    for o in [root] + T.descendants(root):
        for c in list(o.users_collection):
            c.objects.unlink(o)
        col.objects.link(o)
    T.pose_kammi(root, "steht")
    T.drop_to_floor(root)
    T.header("Kammi: fertig")
    yield 0.8
    # .blend sofort sichern (ohne Kontur/Schatten, die setzt jede Szene selbst)
    with bpy.context.temp_override(window=T._win()):
        bpy.ops.wm.save_as_mainfile(filepath=OUT_BLEND, check_existing=False, compress=True, copy=True)
    T.log("kammi.blend gespeichert", OUT_BLEND)
    T.shadow([root], name="Schatten_Kammi", zmax=0.06, pad=1.25)
    yield


T.Run("kammi", build, out=OUT_PNG, res=(800, 800), samples=24, outline_px=2.4, frame_fill=0.8,
      cam=(-72.0, 10.0)).start()
