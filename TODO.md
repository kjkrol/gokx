# Lista rzeczy do poprawy
1. Nalezy przejrzec kod manualnie, bez pomocy AI, urposcic co sie da i probowac zrozumiec "zakamarki". Oraz usunac martwy / niepotrzebny kod.
2. Separacja tworzenia image.RGBA od uzywania tego obrazka (odswiezania i rysowania). Potrzebujemy mechanizmu rejestracji obrazkow.
3. Dzieki temu dla SDL zarejestrowane obrazki stana sie Texutrami w VRAM - uzyskamy pelne wsparcie GPU.
4. Transformacja bedzie wowczas: Transformowac Spatial i Transformowac Surface?